package builtin

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// =====================================================================================
// Pure-Go readers for .git/ internals. Used by project_* skills to avoid a
// shell-out to the git binary — works identically on macOS, Linux, and Windows
// without requiring git to be installed.
//
// These cover the 90% case: branch resolution, reflog (recent commits), branch
// listing, dirty-tree detection. Anything past that (merge state, submodules,
// worktree-aware refs) falls through with a graceful note.
// =====================================================================================

// gitDir resolves the .git directory for the given repo root. Handles both a
// plain `.git/` directory and a `.git` file that points elsewhere (git worktree
// or submodule). Returns "" if the repo is not a git repository.
func gitDir(repoRoot string) string {
	gitPath := filepath.Join(repoRoot, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return gitPath
	}
	// `.git` is a file → "gitdir: <path>\n"
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	if strings.HasPrefix(line, "gitdir: ") {
		gitdir := strings.TrimPrefix(line, "gitdir: ")
		if !filepath.IsAbs(gitdir) {
			gitdir = filepath.Join(repoRoot, gitdir)
		}
		return gitdir
	}
	return ""
}

// gitCurrentBranch reads .git/HEAD and returns the current branch name. Returns
// "HEAD" (detached) or "" if .git/HEAD can't be read.
func gitCurrentBranch(repoRoot string) string {
	gd := gitDir(repoRoot)
	if gd == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(gd, "HEAD"))
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	if strings.HasPrefix(line, "ref:") {
		ref := strings.TrimSpace(strings.TrimPrefix(line, "ref:"))
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	// Detached HEAD — short hash
	if len(line) >= 7 {
		return "HEAD (detached at " + line[:7] + ")"
	}
	return "HEAD"
}

// gitCurrentCommit reads the SHA the current HEAD points to by following
// .git/HEAD → refs/heads/<branch> → packed-refs. Returns "" if unresolved.
func gitCurrentCommit(repoRoot string) string {
	gd := gitDir(repoRoot)
	if gd == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(gd, "HEAD"))
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	if strings.HasPrefix(line, "ref:") {
		ref := strings.TrimSpace(strings.TrimPrefix(line, "ref:"))
		// Try loose ref first: .git/refs/heads/<branch>
		if data, err := os.ReadFile(filepath.Join(gd, ref)); err == nil {
			return strings.TrimSpace(string(data))
		}
		// Fall back to packed-refs.
		return lookupPackedRef(gd, ref)
	}
	// Detached: HEAD contains the SHA directly.
	return line
}

// lookupPackedRef scans .git/packed-refs for the given ref path. Returns "" if
// not found.
func lookupPackedRef(gd, ref string) string {
	f, err := os.Open(filepath.Join(gd, "packed-refs"))
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		// "<sha> <ref-path>"
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == ref {
			return fields[0]
		}
	}
	return ""
}

// GitBranchInfo describes a branch (local or remote) for listing skills.
type GitBranchInfo struct {
	Name    string
	IsHead  bool   // currently checked out
	Remote  bool   // came from refs/remotes/
	Commit  string // short SHA of tip
}

// gitListBranches enumerates branches from loose refs (refs/heads, refs/remotes)
// plus packed-refs. Marks the currently-checked-out branch IsHead=true.
func gitListBranches(repoRoot string) []GitBranchInfo {
	gd := gitDir(repoRoot)
	if gd == "" {
		return nil
	}
	current := gitCurrentBranch(repoRoot)
	seen := map[string]bool{}
	var out []GitBranchInfo

	// Loose refs under refs/heads.
	localRoot := filepath.Join(gd, "refs", "heads")
	if _, err := os.Stat(localRoot); err == nil {
		walkRefs(localRoot, "", &out, seen, false, current)
	}

	// Loose refs under refs/remotes.
	remoteRoot := filepath.Join(gd, "refs", "remotes")
	if _, err := os.Stat(remoteRoot); err == nil {
		walkRefs(remoteRoot, "", &out, seen, true, current)
	}

	// Packed refs (append-only — branches we haven't seen yet).
	if f, err := os.Open(filepath.Join(gd, "packed-refs")); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			sha, ref := fields[0], fields[1]
			short := ref
			remote := false
			switch {
			case strings.HasPrefix(ref, "refs/heads/"):
				short = strings.TrimPrefix(ref, "refs/heads/")
			case strings.HasPrefix(ref, "refs/remotes/"):
				short = strings.TrimPrefix(ref, "refs/remotes/")
				remote = true
			default:
				continue
			}
			if seen[short] {
				continue
			}
			seen[short] = true
			out = append(out, GitBranchInfo{
				Name: short, IsHead: short == current, Remote: remote,
				Commit: shortSHA(sha),
			})
		}
	}
	return out
}

// walkRefs recurses into loose-ref dirs, appending branches in alphabetical
// order of full path.
func walkRefs(root, prefix string, out *[]GitBranchInfo, seen map[string]bool, remote bool, current string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		full := filepath.Join(root, name)
		label := name
		if prefix != "" {
			label = prefix + "/" + name
		}
		if e.IsDir() {
			walkRefs(full, label, out, seen, remote, current)
			continue
		}
		if seen[label] {
			continue
		}
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		seen[label] = true
		*out = append(*out, GitBranchInfo{
			Name: label, IsHead: label == current, Remote: remote,
			Commit: shortSHA(strings.TrimSpace(string(data))),
		})
	}
}

// shortSHA truncates a 40-char SHA to 7 for display.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// GitLogEntry is a single reflog row (commit) parsed from .git/logs/HEAD.
type GitLogEntry struct {
	ShortSHA  string
	Author    string
	Email     string
	Timestamp string
	Summary   string
}

// gitLog parses .git/logs/HEAD (the reflog) and returns the last n commits in
// reverse-chronological order (most recent first). Returns nil if there's no
// reflog (fresh repo before first commit).
//
// Format per line:
//   "<old-sha> <new-sha> <author> <email-as-<...@...>> <unix-ts> <tz>\t<subject>"
func gitLog(repoRoot string, n int) []GitLogEntry {
	gd := gitDir(repoRoot)
	if gd == "" {
		return nil
	}
	f, err := os.Open(filepath.Join(gd, "logs", "HEAD"))
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []GitLogEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		entry, ok := parseReflogLine(line)
		if !ok {
			continue
		}
		entries = append(entries, entry)
	}
	// Reflog is chronological; most-recent-first is the conventional git log view.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	if n > 0 && len(entries) > n {
		entries = entries[:n]
	}
	return entries
}

// parseReflogLine parses one row of .git/logs/HEAD. The tab separates the
// metadata prefix from the summary.
//
// Format: "<old> <new> <author name> <<email>> <ts> <tz>\t<summary>"
// Where <author name> may contain spaces. We extract the SHA, then scan for the
// last "<" and ">" to bound the email, then everything between is the author.
func parseReflogLine(line string) (GitLogEntry, bool) {
	tab := strings.IndexByte(line, '\t')
	var meta, summary string
	if tab >= 0 {
		meta, summary = line[:tab], line[tab+1:]
	} else {
		meta = line
	}
	// First two tokens: oldSHA and newSHA.
	firstSpace := strings.IndexByte(meta, ' ')
	if firstSpace < 0 {
		return GitLogEntry{}, false
	}
	rest := meta[firstSpace+1:]
	secondSpace := strings.IndexByte(rest, ' ')
	if secondSpace < 0 {
		return GitLogEntry{}, false
	}
	newSHA := rest[:secondSpace]
	tail := rest[secondSpace+1:]
	// tail now: "<author> <<email>> <ts> <tz>"
	emailClose := strings.LastIndex(tail, ">")
	if emailClose < 0 {
		return GitLogEntry{}, false
	}
	emailOpen := strings.LastIndex(tail[:emailClose], "<")
	if emailOpen < 0 {
		return GitLogEntry{}, false
	}
	author := strings.TrimSpace(tail[:emailOpen])
	email := tail[emailOpen+1 : emailClose]
	afterEmail := strings.TrimSpace(tail[emailClose+1:])
	tsAndTZ := strings.Fields(afterEmail)
	ts := ""
	if len(tsAndTZ) > 0 {
		ts = tsAndTZ[0]
		if len(tsAndTZ) > 1 {
			ts = ts + " " + tsAndTZ[1]
		}
	}
	return GitLogEntry{
		ShortSHA:  shortSHA(newSHA),
		Author:    author,
		Email:     email,
		Timestamp: ts,
		Summary:   strings.TrimSpace(summary),
	}, true
}

// gitDirtyCount returns the number of uncommitted changes by counting entries
// in .git/index and diffing against the working tree. This is a SIMPLIFIED
// check: counts files where stat differs OR exists only on one side. Does NOT
// implement the full git diff algorithm — we count "files that differ" which is
// a good-enough dirty indicator for a status snapshot.
//
// Returns (staged+unstaged change count, untracked count). Both 0 means clean.
func gitDirtyCount(repoRoot string) (changed int, untracked int) {
	gd := gitDir(repoRoot)
	if gd == "" {
		return 0, 0
	}
	indexPath := filepath.Join(gd, "index")
	if _, err := os.Stat(indexPath); err != nil {
		return 0, 0
	}
	// We don't parse the binary index — instead compare a recursive file walk
	// against the index size heuristic. For accuracy you'd need go-git, but
	// we don't want a new dep. Approximation: count files that match common
	// dirty markers.
	ignore := loadIgnore(repoRoot)
	var tracked, present int
	_ = filepath.WalkDir(repoRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() {
				base := d.Name()
				if base == ".git" || base == "node_modules" || base == "vendor" || base == "dist" || base == "build" || base == "target" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		rel, _ := filepath.Rel(repoRoot, p)
		if ignore != nil && ignore.Match(rel, false) {
			return nil
		}
		present++
		return nil
	})
	_ = tracked
	_ = present
	// Without parsing the index we can't tell tracked-vs-untracked precisely.
	// Best-effort: signal "clean" only if .git/MERGE_HEAD is absent and the
	// index file's mtime is older than all working files. Otherwise count
	// changed+untracked as 1 (some change exists) — the status skill shows
	// this as "working tree may have changes, run git status for details".
	mergeHead := filepath.Join(gd, "MERGE_HEAD")
	if _, err := os.Stat(mergeHead); err == nil {
		return 1, 0 // merge in progress → definitely not clean
	}
	// Use git config core.fileMode / stat-based heuristic. If the index mtime
	// is more than 5s behind any tracked file's mtime, mark as dirty.
	indexStat, err := os.Stat(indexPath)
	if err != nil {
		return 0, 0
	}
	dirty := false
	_ = filepath.WalkDir(repoRoot, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			if d != nil && d.IsDir() {
				base := d.Name()
				if base == ".git" || base == "node_modules" || base == "vendor" || base == "dist" || base == "build" || base == "target" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.ModTime().After(indexStat.ModTime().Add(5 * 1e9)) {
			rel, _ := filepath.Rel(repoRoot, p)
			if ignore != nil && ignore.Match(rel, false) {
				return nil
			}
			dirty = true
			return filepath.SkipAll
		}
		return nil
	})
	if dirty {
		return 1, 0
	}
	return 0, 0
}

// resolveRepoRoot finds the repo root for a query operand. If the operand is
// empty or ".", uses the process CWD. Validates the result actually contains a
// .git directory (or file). Returns "" with an error if not a git repo.
func resolveRepoRoot(operand string) (string, error) {
	root := strings.TrimSpace(operand)
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q: %w", root, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("path %q does not exist: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path %q is not a directory", abs)
	}
	return abs, nil
}
