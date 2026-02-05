package scrub

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type ExportFilter struct {
	rules CompiledRules

	// markMap maps an old mark string like ":12" to a new mark string (or "" for none).
	markMap map[string]string

	// refRewriteMap detects collisions: original ref -> scrubbed ref.
	refRewriteMap map[string]string
	refReverseMap map[string]string

	// replaceHistorySeenFiles tracks which replace_history_with_current files
	// have been "seen" (first occurrence in the history).
	replaceHistorySeenFiles map[string]bool
	// replaceHistoryMarks maps normalized file paths to the synthetic blob marks
	// for replace_history_with_current files.
	replaceHistoryMarks map[string]string
	// nextSyntheticMark is the counter for generating synthetic marks for
	// replace_history_with_current blobs.
	nextSyntheticMark int
	// syntheticBlobsEmitted tracks whether we've emitted synthetic blobs yet.
	syntheticBlobsEmitted bool
}

func NewExportFilter(r CompiledRules) *ExportFilter {
	return &ExportFilter{
		rules:                   r,
		markMap:                 map[string]string{},
		refRewriteMap:           map[string]string{},
		refReverseMap:           map[string]string{},
		replaceHistorySeenFiles: map[string]bool{},
		replaceHistoryMarks:     map[string]string{},
		nextSyntheticMark:       900000000, // Start high to avoid collisions with normal marks
	}
}

func (f *ExportFilter) Filter(r io.Reader, w io.Writer) error {
	br := bufio.NewReader(r)
	bw := bufio.NewWriter(w)
	defer bw.Flush()

	for {
		line, err := br.ReadString('\n')
		if err == io.EOF && line == "" {
			break
		}
		if err != nil && err != io.EOF {
			return err
		}

		switch {
		case line == "blob\n":
			if err := f.handleBlob(br, bw); err != nil {
				return err
			}
		case strings.HasPrefix(line, "commit "):
			// Emit synthetic blobs before the first commit
			if !f.syntheticBlobsEmitted {
				if err := f.emitSyntheticBlobs(bw); err != nil {
					return err
				}
				f.syntheticBlobsEmitted = true
			}
			if err := f.handleCommit(line, br, bw); err != nil {
				return err
			}
		case strings.HasPrefix(line, "tag "):
			if err := f.handleTag(line, br, bw); err != nil {
				return err
			}
		case strings.HasPrefix(line, "reset "):
			if err := f.handleReset(line, br, bw); err != nil {
				return err
			}
		default:
			// progress/checkpoint/etc.
			_, _ = bw.WriteString(line)
		}

		if err == io.EOF {
			break
		}
	}
	return nil
}

// emitSyntheticBlobs emits blob records for all replace_history_with_current files
// at the beginning of the stream, before any commits.
func (f *ExportFilter) emitSyntheticBlobs(bw *bufio.Writer) error {
	files := f.rules.GetReplaceHistoryFiles()
	for _, filePath := range files {
		content := f.rules.GetReplaceHistoryContent(filePath)
		if content == nil {
			// File doesn't exist in HEAD, skip
			continue
		}

		// Generate a synthetic mark for this file
		mark := fmt.Sprintf(":%d", f.nextSyntheticMark)
		f.nextSyntheticMark++
		f.replaceHistoryMarks[filePath] = mark

		// Emit the blob
		_, _ = bw.WriteString("blob\n")
		_, _ = bw.WriteString("mark " + mark + "\n")
		_, _ = bw.WriteString(fmt.Sprintf("data %d\n", len(content)))
		if _, err := bw.Write(content); err != nil {
			return err
		}
		_, _ = bw.WriteString("\n")
	}
	return nil
}

func (f *ExportFilter) handleBlob(br *bufio.Reader, bw *bufio.Writer) error {
	_, _ = bw.WriteString("blob\n")
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return err
		}
		if strings.HasPrefix(line, "data ") {
			n, err := parseDataLen(line)
			if err != nil {
				return err
			}
			b := make([]byte, n)
			if _, err := io.ReadFull(br, b); err != nil {
				return err
			}
			// Consume trailing newline after data payload
			if _, err := br.ReadByte(); err != nil {
				return err
			}
			nb := f.rewriteBytes(b)
			_, _ = bw.WriteString(fmt.Sprintf("data %d\n", len(nb)))
			if _, err := bw.Write(nb); err != nil {
				return err
			}
			_, _ = bw.WriteString("\n")
			return nil
		}
		// pass-through metadata (mark, original-oid)
		_, _ = bw.WriteString(line)
	}
}

func (f *ExportFilter) handleCommit(firstLine string, br *bufio.Reader, bw *bufio.Writer) error {
	origRef := strings.TrimSpace(strings.TrimPrefix(firstLine, "commit "))
	newRef := f.rewriteRef(origRef)
	if err := f.checkRefCollision(origRef, newRef); err != nil {
		return err
	}

	var (
		oldMark       string
		authorLine    string
		committerLine string
		encodingLine  string
		origOidLine   string

		otherHeader []string

		message []byte
		parent  string
		merges  []string
		ops     []string
	)

	// For commits, git fast-export emits:
	//   commit <ref>
	//   mark/author/committer/(encoding)...
	//   data <n>\n
	//   <n bytes of message>
	//   from/merge/file-ops...
	//   \n
	//
	// Important: commit message payload is NOT followed by a delimiter newline; the next command
	// begins immediately after the <n> bytes. Therefore we must not consume an extra byte after
	// reading the payload.
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return err
		}

		switch {
		case strings.HasPrefix(line, "mark "):
			oldMark = strings.TrimSpace(strings.TrimPrefix(line, "mark "))
			f.markMap[oldMark] = oldMark // default unless remapped by skipped commit
		case strings.HasPrefix(line, "original-oid "):
			origOidLine = line
		case strings.HasPrefix(line, "author "):
			authorLine = line
		case strings.HasPrefix(line, "committer "):
			committerLine = line
		case strings.HasPrefix(line, "encoding "):
			encodingLine = line
		case strings.HasPrefix(line, "from "):
			// Some exporters might place from before data; accept it either way.
			parent = strings.TrimSpace(strings.TrimPrefix(line, "from "))
		case strings.HasPrefix(line, "merge "):
			merges = append(merges, strings.TrimSpace(strings.TrimPrefix(line, "merge ")))
		case strings.HasPrefix(line, "data "):
			n, err := parseDataLen(line)
			if err != nil {
				return err
			}
			b := make([]byte, n)
			if _, err := io.ReadFull(br, b); err != nil {
				return err
			}
			message = f.rewriteBytes(b)

			// Parse trailing lines until the blank line ends the commit record.
			for {
				l2, err := br.ReadString('\n')
				if err != nil {
					return err
				}
				if l2 == "\n" {
					break
				}
				if strings.HasPrefix(l2, "from ") {
					parent = strings.TrimSpace(strings.TrimPrefix(l2, "from "))
					continue
				}
				if strings.HasPrefix(l2, "merge ") {
					merges = append(merges, strings.TrimSpace(strings.TrimPrefix(l2, "merge ")))
					continue
				}
				ops = append(ops, l2)
			}
			goto EMIT
		default:
			otherHeader = append(otherHeader, line)
		}
	}

EMIT:
	parentResolved := ""
	if parent != "" {
		parentResolved = f.resolveCommitRef(parent)
	}

	filteredOps, keptOps, err := f.filterOps(ops)
	if err != nil {
		return err
	}

	// Skip commit if exclusions remove all file operations and it's not a merge commit.
	if keptOps == 0 && len(merges) == 0 {
		if oldMark != "" {
			// Any later reference to this mark should resolve to the parent.
			f.markMap[oldMark] = parentResolved
		}
		// Ensure branch tip doesn't advance to a skipped commit.
		_, _ = bw.WriteString("reset " + newRef + "\n")
		if parentResolved != "" {
			_, _ = bw.WriteString("from " + parentResolved + "\n")
		}
		_, _ = bw.WriteString("\n")
		return nil
	}

	// Emit commit record (ordering compatible with git fast-import).
	_, _ = bw.WriteString("commit " + newRef + "\n")
	if oldMark != "" {
		_, _ = bw.WriteString("mark " + oldMark + "\n")
	}
	if origOidLine != "" {
		_, _ = bw.WriteString(origOidLine)
	}
	if authorLine != "" {
		_, _ = bw.WriteString(rewriteIdentityLine("author", authorLine, f.rules))
	}
	if committerLine != "" {
		_, _ = bw.WriteString(rewriteIdentityLine("committer", committerLine, f.rules))
	}
	if encodingLine != "" {
		_, _ = bw.WriteString(encodingLine)
	}
	for _, h := range otherHeader {
		_, _ = bw.WriteString(h)
	}

	// Commit message: do NOT add any delimiter newline after the payload.
	_, _ = bw.WriteString(fmt.Sprintf("data %d\n", len(message)))
	if _, err := bw.Write(message); err != nil {
		return err
	}

	// Parents and merges appear after the message data.
	if parentResolved != "" {
		_, _ = bw.WriteString("from " + parentResolved + "\n")
	}
	for _, m0 := range merges {
		m := f.resolveCommitRef(m0)
		if m == "" {
			continue
		}
		_, _ = bw.WriteString("merge " + m + "\n")
	}

	for _, op := range filteredOps {
		_, _ = bw.WriteString(op)
	}
	_, _ = bw.WriteString("\n")
	return nil
}

func (f *ExportFilter) handleTag(firstLine string, br *bufio.Reader, bw *bufio.Writer) error {
	origRef := strings.TrimSpace(strings.TrimPrefix(firstLine, "tag "))
	newRef := f.rewriteRef(origRef)
	if err := f.checkRefCollision(origRef, newRef); err != nil {
		return err
	}
	_, _ = bw.WriteString("tag " + newRef + "\n")

	var taggerLine, fromLine, markLine, origOidLine string
	var message []byte
	var extra []string

	for {
		line, err := br.ReadString('\n')
		if err == io.EOF && line == "" {
			// Tag is the last record in the stream; treat as end of tag.
			goto EMIT
		}
		if err != nil && err != io.EOF {
			return err
		}
		switch {
		case strings.HasPrefix(line, "from "):
			fromLine = line
		case strings.HasPrefix(line, "mark "):
			markLine = line
		case strings.HasPrefix(line, "original-oid "):
			origOidLine = line
		case strings.HasPrefix(line, "tagger "):
			taggerLine = line
		case strings.HasPrefix(line, "data "):
			n, err := parseDataLen(line)
			if err != nil {
				return err
			}
			b := make([]byte, n)
			if _, err := io.ReadFull(br, b); err != nil {
				return err
			}
			if _, err := br.ReadByte(); err != nil {
				if err == io.EOF {
					// Data payload ends at stream EOF; treat as end of tag.
					message = f.rewriteBytes(b)
					goto EMIT
				}
				return err
			}
			message = f.rewriteBytes(b)
		case line == "\n":
			// end
			goto EMIT
		default:
			extra = append(extra, line)
		}
		if err == io.EOF {
			goto EMIT
		}
	}

EMIT:
	if origOidLine != "" {
		_, _ = bw.WriteString(origOidLine)
	}
	if fromLine != "" {
		// rewrite from ref if it uses marks
		p := strings.TrimSpace(strings.TrimPrefix(fromLine, "from "))
		p2 := f.resolveCommitRef(p)
		if p2 != "" {
			_, _ = bw.WriteString("from " + p2 + "\n")
		}
	}
	if taggerLine != "" {
		// overwrite identity
		_, _ = bw.WriteString(rewriteIdentityLine("tagger", taggerLine, f.rules))
	}
	if markLine != "" {
		_, _ = bw.WriteString(markLine)
	}
	for _, l := range extra {
		_, _ = bw.WriteString(l)
	}
	if message != nil {
		// Tag message: write data payload followed by optional trailing LF.
		// Do NOT add a blank terminator line — unlike commits, tags have no
		// sub-commands after the data section, and an extra blank line causes
		// git fast-import to fail with "Unsupported command: ".
		_, _ = bw.WriteString(fmt.Sprintf("data %d\n", len(message)))
		if _, err := bw.Write(message); err != nil {
			return err
		}
		_, _ = bw.WriteString("\n")
	}
	return nil
}

func (f *ExportFilter) handleReset(firstLine string, br *bufio.Reader, bw *bufio.Writer) error {
	origRef := strings.TrimSpace(strings.TrimPrefix(firstLine, "reset "))
	newRef := f.rewriteRef(origRef)
	if err := f.checkRefCollision(origRef, newRef); err != nil {
		return err
	}
	_, _ = bw.WriteString("reset " + newRef + "\n")

	// In git fast-export, a reset may be followed by:
	//   from <ref>\n
	// or nothing (unborn branch) or the next record immediately.
	// We must not consume the next record if it is not a "from" line.
	peek1, err := br.Peek(1)
	if err != nil {
		return nil // EOF
	}
	if peek1[0] == '\n' {
		// rare blank line
		_, _ = br.ReadByte()
		_, _ = bw.WriteString("\n")
		return nil
	}
	peek5, err := br.Peek(5)
	if err == nil && string(peek5) == "from " {
		line, err := br.ReadString('\n')
		if err != nil {
			return err
		}
		p0 := strings.TrimSpace(strings.TrimPrefix(line, "from "))
		p1 := f.resolveCommitRef(p0)
		if p1 != "" {
			_, _ = bw.WriteString("from " + p1 + "\n")
		} else {
			_, _ = bw.WriteString(line)
		}
	}
	return nil
}

func (f *ExportFilter) filterOps(ops []string) ([]string, int, error) {
	out := make([]string, 0, len(ops))
	kept := 0

	for _, op := range ops {
		opTrim := strings.TrimRight(op, "\n")
		if opTrim == "" {
			continue
		}
		if opTrim == "deleteall" {
			out = append(out, "deleteall\n")
			kept++
			continue
		}
		switch {
		case strings.HasPrefix(opTrim, "M "):
			mode, dataref, path, ok := parseM(opTrim)
			if !ok {
				out = append(out, op)
				kept++
				continue
			}
			if f.rules.ShouldExclude(path) {
				continue
			}
			newPath := f.rules.RewriteString(path)
			newPath = strings.TrimPrefix(newPath, "./")
			if f.rules.ShouldExclude(newPath) {
				continue
			}

			// Check if this is a replace_history_with_current file
			normalizedPath := normPath(newPath)
			if f.rules.ShouldReplaceHistory(normalizedPath) {
				// Check if we have a synthetic mark for this file
				syntheticMark, hasSyntheticMark := f.replaceHistoryMarks[normalizedPath]
				if !hasSyntheticMark {
					// File doesn't exist in HEAD, skip all occurrences
					continue
				}

				if f.replaceHistorySeenFiles[normalizedPath] {
					// Already seen this file, skip this M operation
					// (all subsequent changes are dropped)
					continue
				}
				// First occurrence: emit M with synthetic blob mark
				f.replaceHistorySeenFiles[normalizedPath] = true
				out = append(out, fmt.Sprintf("M %s %s %s\n", mode, syntheticMark, newPath))
				kept++
				continue
			}

			out = append(out, fmt.Sprintf("M %s %s %s\n", mode, dataref, newPath))
			kept++
		case strings.HasPrefix(opTrim, "D "):
			path := strings.TrimSpace(strings.TrimPrefix(opTrim, "D "))
			if f.rules.ShouldExclude(path) {
				continue
			}
			newPath := f.rules.RewriteString(path)
			if f.rules.ShouldExclude(newPath) {
				continue
			}

			// Check if this is a replace_history_with_current file
			normalizedPath := normPath(newPath)
			if f.rules.ShouldReplaceHistory(normalizedPath) {
				// Skip D operations for replace_history files - they should appear
				// to exist unchanged throughout history
				continue
			}

			out = append(out, "D "+newPath+"\n")
			kept++
		case strings.HasPrefix(opTrim, "R "):
			oldP, newP, ok := parseTwoPaths(opTrim, "R")
			if !ok {
				out = append(out, op)
				kept++
				continue
			}
			// If source excluded but dest included, rename cannot be represented safely (fast-import needs source present).
			if f.rules.ShouldExclude(oldP) && !f.rules.ShouldExclude(newP) {
				return nil, 0, fmt.Errorf("unsafe rename from excluded path %q to included path %q; add an exclusion for the destination or avoid renaming excluded files", oldP, newP)
			}
			if f.rules.ShouldExclude(newP) {
				continue
			}
			old2 := f.rules.RewriteString(oldP)
			new2 := f.rules.RewriteString(newP)
			out = append(out, fmt.Sprintf("R %s %s\n", old2, new2))
			kept++
		case strings.HasPrefix(opTrim, "C "):
			oldP, newP, ok := parseTwoPaths(opTrim, "C")
			if !ok {
				out = append(out, op)
				kept++
				continue
			}
			if f.rules.ShouldExclude(oldP) && !f.rules.ShouldExclude(newP) {
				return nil, 0, fmt.Errorf("unsafe copy from excluded path %q to included path %q; add an exclusion for the destination or avoid copying excluded files", oldP, newP)
			}
			if f.rules.ShouldExclude(newP) {
				continue
			}
			old2 := f.rules.RewriteString(oldP)
			new2 := f.rules.RewriteString(newP)
			out = append(out, fmt.Sprintf("C %s %s\n", old2, new2))
			kept++
		default:
			// Unknown operation; keep but scrub obvious usernames in the line
			out = append(out, f.rules.RewriteString(op))
			kept++
		}
	}
	return out, kept, nil
}

func (f *ExportFilter) resolveCommitRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if strings.HasPrefix(ref, ":") {
		if v, ok := f.markMap[ref]; ok {
			return v
		}
		return ref
	}
	// sha or refname; return as-is
	return ref
}

func (f *ExportFilter) rewriteBytes(b []byte) []byte {
	return f.rules.RewriteBytes(b)
}

func rewriteIdentityLine(kind, line string, rules CompiledRules) string {
	// line format: "<kind> Name <email> timestamp tz\n"
	// We keep timestamp+tz from original, but overwrite name/email.
	rest := strings.TrimSpace(strings.TrimPrefix(line, kind+" "))
	// Find ">" that ends email
	i := strings.LastIndex(rest, ">")
	if i == -1 {
		// fallback: just rewrite string
		return kind + " " + rules.RewriteString(rest) + "\n"
	}
	after := strings.TrimSpace(rest[i+1:]) // timestamp tz
	name := rules.PublicAuthorName()
	email := rules.PublicAuthorEmail()
	return fmt.Sprintf("%s %s <%s> %s\n", kind, name, email, after)
}

func parseDataLen(line string) (int, error) {
	s := strings.TrimSpace(strings.TrimPrefix(line, "data "))
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid data length: %q", line)
	}
	return n, nil
}

func parseM(line string) (mode, dataref, path string, ok bool) {
	// line: "M <mode> <dataref> <path>"
	// Find first space after "M "
	rest := strings.TrimPrefix(line, "M ")
	i1 := strings.IndexByte(rest, ' ')
	if i1 < 0 {
		return "", "", "", false
	}
	mode = rest[:i1]
	rest2 := rest[i1+1:]
	i2 := strings.IndexByte(rest2, ' ')
	if i2 < 0 {
		return "", "", "", false
	}
	dataref = rest2[:i2]
	path = strings.TrimSpace(rest2[i2+1:])
	return mode, dataref, path, true
}

func parseTwoPaths(line, prefix string) (a, b string, ok bool) {
	// Very common case: paths without spaces. If paths contain spaces, fast-import format becomes ambiguous.
	parts := strings.Fields(line)
	if len(parts) != 3 || parts[0] != prefix {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func (f *ExportFilter) rewriteRef(ref string) string {
	// ref is a token like "refs/heads/main" or "refs/tags/v1.0"
	return f.rules.RewriteString(ref)
}

func (f *ExportFilter) checkRefCollision(orig, rewritten string) error {
	if prev, ok := f.refRewriteMap[orig]; ok {
		if prev != rewritten {
			return fmt.Errorf("internal ref rewrite mismatch for %q", orig)
		}
		return nil
	}
	f.refRewriteMap[orig] = rewritten
	if back, ok := f.refReverseMap[rewritten]; ok && back != orig {
		return fmt.Errorf("ref name collision after scrubbing: %q and %q both become %q", back, orig, rewritten)
	}
	f.refReverseMap[rewritten] = orig
	return nil
}
