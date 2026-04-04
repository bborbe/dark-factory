// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prompt

import (
	"bufio"
	"bytes"
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/adrg/frontmatter"
	"github.com/bborbe/collection"
	"github.com/bborbe/errors"
	libtime "github.com/bborbe/time"
	"github.com/bborbe/validation"
	"gopkg.in/yaml.v3"

	"github.com/bborbe/dark-factory/pkg/specnum"
)

// ErrEmptyPrompt is returned when a prompt file is empty or contains only whitespace.
var ErrEmptyPrompt = stderrors.New("prompt file is empty")

var (
	validPatternRegexp        = regexp.MustCompile(`^(\d{3})-(.+)\.md$`)
	numericPatternRegexp      = regexp.MustCompile(`^(\d+)-(.+)\.md$`)
	hasNumberPrefixRegexp     = regexp.MustCompile(`^\d{3}-`)
	extractNumberPrefixRegexp = regexp.MustCompile(`^(\d{3})-`)
	anyNumberPrefixRegexp     = regexp.MustCompile(`^\d+-`)
)

// StripNumberPrefix removes any leading numeric prefix (e.g. "200-foo.md" → "foo.md", "1-bar.md" → "bar.md").
// Returns the original filename if no numeric prefix is found.
func StripNumberPrefix(filename string) string {
	return anyNumberPrefixRegexp.ReplaceAllString(filename, "")
}

// PromptStatus represents the current state of a prompt.
//
//nolint:revive // PromptStatus is the intended name per go-enum-type-pattern
type PromptStatus string

const (
	// IdeaPromptStatus indicates a rough concept that needs refinement before it can be reviewed.
	IdeaPromptStatus PromptStatus = "idea"
	// DraftPromptStatus indicates the prompt is complete and ready for human review and approval.
	DraftPromptStatus PromptStatus = "draft"
	// ApprovedPromptStatus indicates the prompt has been approved and queued for execution.
	ApprovedPromptStatus PromptStatus = "approved"
	// ExecutingPromptStatus indicates the prompt is currently being executed in a YOLO container.
	ExecutingPromptStatus PromptStatus = "executing"
	// CompletedPromptStatus indicates the prompt has been executed successfully.
	CompletedPromptStatus PromptStatus = "completed"
	// FailedPromptStatus indicates the prompt execution failed and needs fix or retry.
	FailedPromptStatus PromptStatus = "failed"
	// InReviewPromptStatus indicates the prompt's PR is under review.
	InReviewPromptStatus PromptStatus = "in_review"
	// PendingVerificationPromptStatus indicates the prompt is awaiting verification after review.
	PendingVerificationPromptStatus PromptStatus = "pending_verification"
	// CancelledPromptStatus indicates the prompt was cancelled before or during execution.
	CancelledPromptStatus PromptStatus = "cancelled"
	// PermanentlyFailedPromptStatus indicates the prompt exhausted all auto-retries and will not be retried automatically.
	PermanentlyFailedPromptStatus PromptStatus = "permanently_failed"
)

// AvailablePromptStatuses is the collection of all valid PromptStatus values.
var AvailablePromptStatuses = PromptStatuses{
	IdeaPromptStatus,
	DraftPromptStatus,
	ApprovedPromptStatus,
	ExecutingPromptStatus,
	CompletedPromptStatus,
	FailedPromptStatus,
	InReviewPromptStatus,
	PendingVerificationPromptStatus,
	CancelledPromptStatus,
	PermanentlyFailedPromptStatus,
}

// PromptStatuses is a slice of PromptStatus values.
//
//nolint:revive // PromptStatuses is the intended name per go-enum-type-pattern
type PromptStatuses []PromptStatus

// Contains returns true if the given status is in the collection.
func (p PromptStatuses) Contains(status PromptStatus) bool {
	return collection.Contains(p, status)
}

// String returns the string representation of the PromptStatus.
func (s PromptStatus) String() string { return string(s) }

// Validate validates the PromptStatus value.
func (s PromptStatus) Validate(ctx context.Context) error {
	if !AvailablePromptStatuses.Contains(s) {
		return errors.Wrapf(ctx, validation.Error, "status(%s) is invalid", s)
	}
	return nil
}

// Rename represents a file rename operation.
type Rename struct {
	OldPath string
	NewPath string
}

// Prompt represents a prompt file with YAML frontmatter.
type Prompt struct {
	Path   string
	Status PromptStatus
}

// Validate validates the Prompt struct.
func (p Prompt) Validate(ctx context.Context) error {
	return validation.All{
		validation.Name("path", validation.NotEmptyString(p.Path)),
		validation.Name("status", p.Status),
		validation.Name("filename", validation.HasValidationFunc(func(ctx context.Context) error {
			if !hasNumberPrefix(filepath.Base(p.Path)) {
				return errors.Errorf(ctx, "missing NNN- prefix: %s", filepath.Base(p.Path))
			}
			return nil
		})),
	}.Validate(ctx)
}

// ValidateForExecution validates that a prompt is ready to execute.
func (p Prompt) ValidateForExecution(ctx context.Context) error {
	return validation.All{
		validation.Name("prompt", p),
		validation.Name("status", validation.HasValidationFunc(func(ctx context.Context) error {
			if p.Status != ApprovedPromptStatus {
				return errors.Errorf(ctx, "expected status approved, got %s", p.Status)
			}
			return nil
		})),
	}.Validate(ctx)
}

// Number extracts the numeric prefix from the prompt filename.
// Returns -1 if the filename has no numeric prefix.
func (p Prompt) Number() int {
	return extractNumberFromFilename(filepath.Base(p.Path))
}

// SpecList is a []string that handles both YAML scalar and sequence for the spec field.
// This enables backward compatibility where spec was a single string (e.g. spec: "017")
// while also supporting multiple specs (e.g. spec: ["017", "019"]).
type SpecList []string

// UnmarshalYAML implements yaml.Unmarshaler to accept both scalar and sequence.
func (s *SpecList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		if value.Value != "" {
			*s = SpecList{value.Value}
		}
		return nil
	case yaml.SequenceNode:
		var slice []string
		if err := value.Decode(&slice); err != nil {
			return err
		}
		*s = SpecList(slice)
		return nil
	default:
		return fmt.Errorf("unexpected YAML node kind for spec: %v", value.Kind)
	}
}

// Frontmatter represents the YAML frontmatter in a prompt file.
type Frontmatter struct {
	Status             string   `yaml:"status"`
	Specs              SpecList `yaml:"spec,omitempty,flow"`
	Summary            string   `yaml:"summary,omitempty"`
	Container          string   `yaml:"container,omitempty"`
	DarkFactoryVersion string   `yaml:"dark-factory-version,omitempty"`
	Created            string   `yaml:"created,omitempty"`
	Queued             string   `yaml:"queued,omitempty"`
	Started            string   `yaml:"started,omitempty"`
	Completed          string   `yaml:"completed,omitempty"`
	PRURL              string   `yaml:"pr-url,omitempty"`
	Branch             string   `yaml:"branch,omitempty"`
	Issue              string   `yaml:"issue,omitempty"`
	RetryCount         int      `yaml:"retryCount,omitempty"`
	LastFailReason     string   `yaml:"lastFailReason,omitempty"`
}

// HasSpec returns true if the given spec ID is in the Specs list.
// Comparison is by parsed integer prefix: "019" matches "19", "0019", and "019-review-fix-loop".
// If either value has no numeric prefix, falls back to exact string match.
func (f Frontmatter) HasSpec(id string) bool {
	idNum := specnum.Parse(id)
	for _, s := range f.Specs {
		if idNum >= 0 {
			if specnum.Parse(s) == idNum {
				return true
			}
		} else {
			if s == id {
				return true
			}
		}
	}
	return false
}

// PromptFile represents a loaded prompt file with immutable body and mutable frontmatter.
//
//nolint:revive // PromptFile is the intended name per requirements
type PromptFile struct {
	Path                  string
	Frontmatter           Frontmatter
	Body                  []byte // immutable after Load — never modified
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewPromptFile creates a PromptFile with the given fields and currentDateTimeGetter.
// This is intended for use in tests where a PromptFile must be constructed without reading from disk.
func NewPromptFile(
	path string,
	fm Frontmatter,
	body []byte,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) *PromptFile {
	return &PromptFile{
		Path:                  path,
		Frontmatter:           fm,
		Body:                  body,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// Load reads a prompt file from disk, parsing frontmatter and body.
// Body is stored as-is and never modified by Save.
func Load(
	ctx context.Context,
	path string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (*PromptFile, error) {
	// #nosec G304 -- path is from ListQueued which scans prompts directory
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "read file")
	}

	var fm Frontmatter
	yamlV3Format := frontmatter.NewFormat("---", "---", yaml.Unmarshal)
	body, err := frontmatter.Parse(bytes.NewReader(content), &fm, yamlV3Format)
	if err != nil {
		// No frontmatter — entire file is body
		pf := &PromptFile{
			Path:                  path,
			Body:                  content,
			currentDateTimeGetter: currentDateTimeGetter,
		}
		slog.Debug("file loaded", "path", path, "bodySize", len(content), "hasStatus", false)
		return pf, nil
	}

	pf := &PromptFile{
		Path:                  path,
		Frontmatter:           fm,
		Body:                  body,
		currentDateTimeGetter: currentDateTimeGetter,
	}
	slog.Debug("file loaded", "path", path, "bodySize", len(body), "hasStatus", fm.Status != "")
	return pf, nil
}

// Save writes the prompt file back to disk: frontmatter + body.
// Body is always preserved exactly as loaded.
func (pf *PromptFile) Save(ctx context.Context) error {
	fm, err := yaml.Marshal(&pf.Frontmatter)
	if err != nil {
		return errors.Wrap(ctx, err, "marshal frontmatter")
	}

	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(fm)
	buf.WriteString("---\n")
	buf.Write(pf.Body)

	if err := os.WriteFile(pf.Path, buf.Bytes(), 0600); err != nil {
		return errors.Wrap(ctx, err, "write file")
	}

	slog.Debug(
		"file saved",
		"path",
		pf.Path,
		"bodySize",
		len(pf.Body),
		"status",
		pf.Frontmatter.Status,
	)
	return nil
}

// Content returns the body as a string, stripped of leading empty frontmatter blocks.
// Returns ErrEmptyPrompt if body is empty or whitespace-only.
func (pf *PromptFile) Content() (string, error) {
	result := strings.TrimSpace(string(pf.Body))
	if len(result) == 0 {
		return "", ErrEmptyPrompt
	}
	return string(pf.Body), nil
}

// Title extracts the first # heading from the body.
func (pf *PromptFile) Title() string {
	scanner := bufio.NewScanner(bytes.NewReader(pf.Body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

// now returns the current time from the injected getter.
func (pf *PromptFile) now() time.Time {
	return time.Time(pf.currentDateTimeGetter.Now())
}

// PrepareForExecution sets all fields needed before container launch.
// This replaces separate SetContainer + SetVersion + SetStatus calls.
func (pf *PromptFile) PrepareForExecution(container, version string) {
	now := pf.now().UTC().Format(time.RFC3339)
	pf.Frontmatter.Status = string(ExecutingPromptStatus)
	pf.Frontmatter.Container = container
	pf.Frontmatter.DarkFactoryVersion = version
	pf.Frontmatter.Started = now
	// Ensure created/queued timestamps exist
	if pf.Frontmatter.Created == "" {
		pf.Frontmatter.Created = now
	}
	if pf.Frontmatter.Queued == "" {
		pf.Frontmatter.Queued = now
	}
}

// MarkCompleted sets status to completed with timestamp.
func (pf *PromptFile) MarkCompleted() {
	pf.Frontmatter.Status = string(CompletedPromptStatus)
	pf.Frontmatter.Completed = pf.now().UTC().Format(time.RFC3339)
}

// MarkFailed sets status to failed with timestamp.
func (pf *PromptFile) MarkFailed() {
	pf.Frontmatter.Status = string(FailedPromptStatus)
	pf.Frontmatter.Completed = pf.now().UTC().Format(time.RFC3339)
}

// SetLastFailReason records the human-readable reason for the last failure.
func (pf *PromptFile) SetLastFailReason(reason string) {
	pf.Frontmatter.LastFailReason = reason
}

// MarkPermanentlyFailed sets status to permanently_failed and records the reason.
func (pf *PromptFile) MarkPermanentlyFailed(reason string) {
	pf.Frontmatter.Status = string(PermanentlyFailedPromptStatus)
	pf.Frontmatter.Completed = pf.now().UTC().Format(time.RFC3339)
	pf.Frontmatter.LastFailReason = reason
}

// MarkPendingVerification sets status to pending_verification.
func (pf *PromptFile) MarkPendingVerification() {
	pf.Frontmatter.Status = string(PendingVerificationPromptStatus)
}

// MarkCancelled sets status to cancelled.
func (pf *PromptFile) MarkCancelled() {
	pf.Frontmatter.Status = string(CancelledPromptStatus)
}

// MarkApproved sets status to approved and ensures created/queued timestamps exist.
func (pf *PromptFile) MarkApproved() {
	now := pf.now().UTC().Format(time.RFC3339)
	pf.Frontmatter.Status = string(ApprovedPromptStatus)
	if pf.Frontmatter.Created == "" {
		pf.Frontmatter.Created = now
	}
	if pf.Frontmatter.Queued == "" {
		pf.Frontmatter.Queued = now
	}
}

// VerificationSection extracts the content of the <verification> tag from the prompt body.
// Returns an empty string if no <verification> tag is found.
func (pf *PromptFile) VerificationSection() string {
	body := string(pf.Body)
	const openTag = "<verification>"
	const closeTag = "</verification>"
	start := strings.Index(body, openTag)
	if start == -1 {
		return ""
	}
	start += len(openTag)
	end := strings.Index(body[start:], closeTag)
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(body[start : start+end])
}

// SetSummary sets the summary field in frontmatter.
func (pf *PromptFile) SetSummary(summary string) {
	pf.Frontmatter.Summary = summary
}

// PRURL returns the pr-url field from frontmatter.
func (pf *PromptFile) PRURL() string {
	return pf.Frontmatter.PRURL
}

// SetPRURL sets the pr-url field in frontmatter.
func (pf *PromptFile) SetPRURL(url string) {
	pf.Frontmatter.PRURL = url
}

// RetryCount returns the retryCount field from frontmatter.
func (pf *PromptFile) RetryCount() int {
	return pf.Frontmatter.RetryCount
}

// Branch returns the branch field from frontmatter.
func (pf *PromptFile) Branch() string {
	return pf.Frontmatter.Branch
}

// SetBranch sets the branch field in frontmatter.
func (pf *PromptFile) SetBranch(branch string) {
	pf.Frontmatter.Branch = branch
}

// SetBranchIfEmpty sets the branch field only if it is currently empty.
func (pf *PromptFile) SetBranchIfEmpty(branch string) {
	if pf.Frontmatter.Branch == "" {
		pf.Frontmatter.Branch = branch
	}
}

// SetIssue sets the issue field in frontmatter.
func (pf *PromptFile) SetIssue(issue string) {
	pf.Frontmatter.Issue = issue
}

// SetIssueIfEmpty sets the issue field only if it is currently empty.
func (pf *PromptFile) SetIssueIfEmpty(issue string) {
	if pf.Frontmatter.Issue == "" {
		pf.Frontmatter.Issue = issue
	}
}

// Issue returns the issue tracker reference from the prompt frontmatter (empty string if unset).
func (pf *PromptFile) Issue() string {
	return pf.Frontmatter.Issue
}

// Specs returns the specs slice from frontmatter. Returns an empty slice if nil.
func (pf *PromptFile) Specs() []string {
	if pf.Frontmatter.Specs == nil {
		return []string{}
	}
	return []string(pf.Frontmatter.Specs)
}

//counterfeiter:generate -o ../../mocks/file-mover.go --fake-name FileMover . FileMover

// FileMover handles file move operations with git awareness.
type FileMover interface {
	MoveFile(ctx context.Context, oldPath string, newPath string) error
}

//counterfeiter:generate -o ../../mocks/prompt-manager.go --fake-name Manager . Manager

// Manager manages prompt file operations.
type Manager interface {
	ResetExecuting(ctx context.Context) error
	ResetFailed(ctx context.Context) error
	HasExecuting(ctx context.Context) bool
	ListQueued(ctx context.Context) ([]Prompt, error)
	Load(ctx context.Context, path string) (*PromptFile, error)
	ReadFrontmatter(ctx context.Context, path string) (*Frontmatter, error)
	SetStatus(ctx context.Context, path string, status string) error
	SetContainer(ctx context.Context, path string, name string) error
	SetVersion(ctx context.Context, path string, version string) error
	SetPRURL(ctx context.Context, path string, url string) error
	SetBranch(ctx context.Context, path string, branch string) error
	IncrementRetryCount(ctx context.Context, path string) error
	Content(ctx context.Context, path string) (string, error)
	Title(ctx context.Context, path string) (string, error)
	MoveToCompleted(ctx context.Context, path string) error
	NormalizeFilenames(ctx context.Context, dir string) ([]Rename, error)
	AllPreviousCompleted(ctx context.Context, n int) bool
	FindMissingCompleted(ctx context.Context, n int) []int
	FindPromptStatusInProgress(ctx context.Context, number int) string
	// HasQueuedPromptsOnBranch returns true if any queued prompt (other than excludePath)
	// has the given branch value in its frontmatter.
	HasQueuedPromptsOnBranch(ctx context.Context, branch string, excludePath string) (bool, error)
}

// NewManager creates a new Manager.
func NewManager(
	inboxDir string,
	inProgressDir string,
	completedDir string,
	mover FileMover,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) Manager {
	return &manager{
		inboxDir:              inboxDir,
		inProgressDir:         inProgressDir,
		completedDir:          completedDir,
		mover:                 mover,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// manager implements Manager.
type manager struct {
	inboxDir              string
	inProgressDir         string
	completedDir          string
	mover                 FileMover
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// ResetExecuting resets any prompts with status "executing" back to "approved".
func (pm *manager) ResetExecuting(ctx context.Context) error {
	return ResetExecuting(ctx, pm.inProgressDir, pm.currentDateTimeGetter)
}

// ResetFailed resets any prompts with status "failed" back to "approved".
func (pm *manager) ResetFailed(ctx context.Context) error {
	return ResetFailed(ctx, pm.inProgressDir, pm.currentDateTimeGetter)
}

// HasExecuting returns true if any prompt in dir has status "executing".
func (pm *manager) HasExecuting(ctx context.Context) bool {
	return HasExecuting(ctx, pm.inProgressDir, pm.currentDateTimeGetter)
}

// ListQueued scans a directory for .md files that should be picked up.
func (pm *manager) ListQueued(ctx context.Context) ([]Prompt, error) {
	return ListQueued(ctx, pm.inProgressDir, pm.currentDateTimeGetter)
}

// Load reads a prompt file from disk, parsing frontmatter and body.
func (pm *manager) Load(ctx context.Context, path string) (*PromptFile, error) {
	return Load(ctx, path, pm.currentDateTimeGetter)
}

// ReadFrontmatter reads frontmatter from a file.
func (pm *manager) ReadFrontmatter(ctx context.Context, path string) (*Frontmatter, error) {
	return ReadFrontmatter(ctx, path, pm.currentDateTimeGetter)
}

// SetStatus updates the status field in a prompt file's frontmatter.
func (pm *manager) SetStatus(ctx context.Context, path string, status string) error {
	return SetStatus(ctx, path, status, pm.currentDateTimeGetter)
}

// SetContainer updates the container field in a prompt file's frontmatter.
func (pm *manager) SetContainer(ctx context.Context, path string, name string) error {
	return SetContainer(ctx, path, name, pm.currentDateTimeGetter)
}

// SetVersion updates the dark-factory-version field in a prompt file's frontmatter.
func (pm *manager) SetVersion(ctx context.Context, path string, version string) error {
	return SetVersion(ctx, path, version, pm.currentDateTimeGetter)
}

// SetPRURL updates the pr-url field in a prompt file's frontmatter.
func (pm *manager) SetPRURL(ctx context.Context, path string, url string) error {
	return SetPRURL(ctx, path, url, pm.currentDateTimeGetter)
}

// SetBranch updates the branch field in a prompt file's frontmatter.
func (pm *manager) SetBranch(ctx context.Context, path string, branch string) error {
	return SetBranch(ctx, path, branch, pm.currentDateTimeGetter)
}

// IncrementRetryCount increments the retryCount field in a prompt file's frontmatter.
func (pm *manager) IncrementRetryCount(ctx context.Context, path string) error {
	return IncrementRetryCount(ctx, path, pm.currentDateTimeGetter)
}

// Content returns the prompt content (without frontmatter) for passing to Docker.
func (pm *manager) Content(ctx context.Context, path string) (string, error) {
	return Content(ctx, path, pm.currentDateTimeGetter)
}

// Title extracts the first # heading from a prompt file.
func (pm *manager) Title(ctx context.Context, path string) (string, error) {
	return Title(ctx, path, pm.currentDateTimeGetter)
}

// MoveToCompleted sets status to "completed" and moves a prompt file to the completed/ subdirectory.
func (pm *manager) MoveToCompleted(ctx context.Context, path string) error {
	return MoveToCompleted(ctx, path, pm.completedDir, pm.mover, pm.currentDateTimeGetter)
}

// NormalizeFilenames scans a directory for .md files and ensures they follow the NNN-slug.md naming convention.
// It also checks the completed directory for used numbers.
func (pm *manager) NormalizeFilenames(ctx context.Context, dir string) ([]Rename, error) {
	return NormalizeFilenames(ctx, dir, pm.completedDir, pm.mover)
}

// AllPreviousCompleted checks if all prompts with numbers less than n are in completed/.
func (pm *manager) AllPreviousCompleted(ctx context.Context, n int) bool {
	return AllPreviousCompleted(ctx, pm.completedDir, n)
}

// FindMissingCompleted returns prompt numbers less than n that are NOT in completed/.
func (pm *manager) FindMissingCompleted(ctx context.Context, n int) []int {
	return FindMissingCompleted(ctx, pm.completedDir, n)
}

// FindPromptStatusInProgress looks up a prompt by number in the in-progress directory and returns its frontmatter status.
func (pm *manager) FindPromptStatusInProgress(ctx context.Context, number int) string {
	return FindPromptStatus(ctx, pm.inProgressDir, number)
}

// HasQueuedPromptsOnBranch returns true if any queued prompt (other than excludePath)
// has the given branch value in its frontmatter.
func (pm *manager) HasQueuedPromptsOnBranch(
	ctx context.Context,
	branch string,
	excludePath string,
) (bool, error) {
	queued, err := pm.ListQueued(ctx)
	if err != nil {
		return false, errors.Wrap(ctx, err, "list queued prompts")
	}
	for _, p := range queued {
		if p.Path == excludePath {
			continue
		}
		pf, err := pm.Load(ctx, p.Path)
		if err != nil {
			slog.Warn("failed to load prompt for branch check", "path", p.Path, "error", err)
			continue
		}
		if pf.Branch() == branch {
			return true, nil
		}
	}
	return false, nil
}

// ListQueued scans a directory for .md files that should be picked up.
// Files are picked up UNLESS they have an explicit skip status (executing, completed, failed).
// Sorted alphabetically by filename.
func ListQueued(
	ctx context.Context,
	dir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) ([]Prompt, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "read directory")
	}

	queued := make([]Prompt, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		fm, err := readFrontmatter(ctx, path, currentDateTimeGetter)
		if err != nil {
			// Skip files with read errors
			slog.Warn("skipping prompt", "file", entry.Name(), "error", err)
			continue
		}

		// Skip files with explicit skip status
		if fm.Status == string(ExecutingPromptStatus) ||
			fm.Status == string(CompletedPromptStatus) ||
			fm.Status == string(FailedPromptStatus) ||
			fm.Status == string(InReviewPromptStatus) ||
			fm.Status == string(PendingVerificationPromptStatus) {
			slog.Debug("skipping prompt", "file", entry.Name(), "status", fm.Status)
			continue
		}

		// Normalize status to "approved" for consistency
		status := PromptStatus(fm.Status)
		if fm.Status == "" {
			status = ApprovedPromptStatus
		}
		queued = append(queued, Prompt{
			Path:   path,
			Status: status,
		})
	}

	// Sort alphabetically by filename
	sort.Slice(queued, func(i, j int) bool {
		return filepath.Base(queued[i].Path) < filepath.Base(queued[j].Path)
	})

	return queued, nil
}

// ResetExecuting resets any prompts with status "executing" back to "approved".
// This handles prompts that got stuck from a previous crash.
func ResetExecuting(
	ctx context.Context,
	dir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return errors.Wrap(ctx, err, "read directory")
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		pf, err := Load(ctx, path, currentDateTimeGetter)
		if err != nil {
			continue
		}

		if pf.Frontmatter.Status == string(ExecutingPromptStatus) {
			pf.MarkApproved()
			if err := pf.Save(ctx); err != nil {
				return errors.Wrap(ctx, err, "reset executing prompt")
			}
		}
	}

	return nil
}

// ResetFailed resets any prompts with status "failed" back to "approved".
// This allows the factory to retry failed prompts after a restart.
func ResetFailed(
	ctx context.Context,
	dir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return errors.Wrap(ctx, err, "read directory")
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		pf, err := Load(ctx, path, currentDateTimeGetter)
		if err != nil {
			continue
		}

		if pf.Frontmatter.Status == string(FailedPromptStatus) {
			pf.MarkApproved()
			if err := pf.Save(ctx); err != nil {
				return errors.Wrap(ctx, err, "reset failed prompt")
			}
			slog.Info("reset failed prompt to approved", "file", entry.Name())
		}
	}

	return nil
}

// SetStatus updates the status field in a prompt file's frontmatter.
// If the file has no frontmatter, adds frontmatter with the status field.
// Also sets appropriate timestamp fields based on status.
func SetStatus(
	ctx context.Context,
	path string,
	status string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	pf, err := Load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	oldStatus := pf.Frontmatter.Status
	now := pf.now().UTC().Format(time.RFC3339)
	pf.Frontmatter.Status = status
	slog.Debug("status changed", "path", path, "oldStatus", oldStatus, "newStatus", status)

	// Set timestamps based on status
	switch PromptStatus(status) {
	case ApprovedPromptStatus:
		// Set queued timestamp only if empty (preserve original on retry)
		if pf.Frontmatter.Queued == "" {
			pf.Frontmatter.Queued = now
		}
		// Ensure created timestamp exists
		if pf.Frontmatter.Created == "" {
			pf.Frontmatter.Created = now
		}
	case ExecutingPromptStatus:
		// Always overwrite started time (retries get fresh start)
		pf.Frontmatter.Started = now
	case CompletedPromptStatus, FailedPromptStatus:
		// Always overwrite completed time
		pf.Frontmatter.Completed = now
	}

	return pf.Save(ctx)
}

// SetContainer updates the container field in a prompt file's frontmatter.
// If the file has no frontmatter, adds frontmatter with the container field.
func SetContainer(
	ctx context.Context,
	path string,
	container string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	pf, err := Load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.Frontmatter.Container = container
	return pf.Save(ctx)
}

// SetVersion updates the dark-factory-version field in a prompt file's frontmatter.
// If the file has no frontmatter, adds frontmatter with the version field.
func SetVersion(
	ctx context.Context,
	path string,
	version string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	pf, err := Load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.Frontmatter.DarkFactoryVersion = version
	return pf.Save(ctx)
}

// SetPRURL updates the pr-url field in a prompt file's frontmatter.
// If the file has no frontmatter, adds frontmatter with the pr-url field.
func SetPRURL(
	ctx context.Context,
	path string,
	url string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	pf, err := Load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.SetPRURL(url)
	return pf.Save(ctx)
}

// SetBranch updates the branch field in a prompt file's frontmatter.
// If the file has no frontmatter, adds frontmatter with the branch field.
func SetBranch(
	ctx context.Context,
	path string,
	branch string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	pf, err := Load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.SetBranch(branch)
	return pf.Save(ctx)
}

// IncrementRetryCount increments the retryCount field in a prompt file's frontmatter by 1.
// If the file has no frontmatter, adds frontmatter with retryCount set to 1.
func IncrementRetryCount(
	ctx context.Context,
	path string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	pf, err := Load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.Frontmatter.RetryCount++
	return pf.Save(ctx)
}

// Title extracts the first # heading from a prompt file.
// Handles files with or without frontmatter.
// If no heading is found, returns the filename without extension.
func Title(
	ctx context.Context,
	path string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (string, error) {
	pf, err := Load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return "", errors.Wrap(ctx, err, "load prompt")
	}

	title := pf.Title()
	if title == "" {
		// No heading found - use filename without extension
		filename := filepath.Base(path)
		return strings.TrimSuffix(filename, ".md"), nil
	}

	return title, nil
}

// Content returns the prompt content (without frontmatter) for passing to Docker.
// Returns ErrEmptyPrompt if the file is empty or contains only whitespace.
func Content(
	ctx context.Context,
	path string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (string, error) {
	pf, err := Load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return "", errors.Wrap(ctx, err, "load prompt")
	}

	return pf.Content()
}

// MoveToCompleted sets status to "completed" and moves a prompt file to the completed directory.
// This ensures files in completed/ always have the correct status.
func MoveToCompleted(
	ctx context.Context,
	path string,
	completedDir string,
	mover FileMover,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	// Load, mark completed, and save before moving
	pf, err := Load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.MarkCompleted()
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "set completed status")
	}

	// Ensure completed directory exists
	if err := os.MkdirAll(completedDir, 0750); err != nil {
		return errors.Wrap(ctx, err, "create completed directory")
	}

	// Move file
	filename := filepath.Base(path)
	dest := filepath.Join(completedDir, filename)

	slog.Debug("moving to completed", "from", path, "to", dest)

	if err := mover.MoveFile(ctx, path, dest); err != nil {
		return errors.Wrap(ctx, err, "move file")
	}

	return nil
}

// ReadFrontmatter reads frontmatter from a file.
func ReadFrontmatter(
	ctx context.Context,
	path string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (*Frontmatter, error) {
	return readFrontmatter(ctx, path, currentDateTimeGetter)
}

// HasExecuting returns true if any prompt in dir has status "executing".
func HasExecuting(
	ctx context.Context,
	dir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		fm, err := readFrontmatter(ctx, filepath.Join(dir, entry.Name()), currentDateTimeGetter)
		if err != nil {
			continue
		}
		if fm.Status == string(ExecutingPromptStatus) {
			return true
		}
	}
	return false
}

// fileInfo represents information about a prompt file.
type fileInfo struct {
	name   string
	number int
	slug   string
}

// NormalizeFilenames scans a directory for .md files and ensures they follow the NNN-slug.md naming convention.
// Files are renamed if they:
// - Have no numeric prefix (gets next available number)
// - Have a duplicate number (later file gets next available number)
// - Have wrong format (e.g., 9-foo.md instead of 009-foo.md)
// Returns list of renames performed.
func NormalizeFilenames(
	ctx context.Context,
	dir string,
	completedDir string,
	mover FileMover,
) ([]Rename, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "read directory")
	}

	files, usedNumbers := scanPromptFiles(entries)

	// Also collect numbers used in completed/ so we don't assign duplicates.
	completedEntries, err := os.ReadDir(completedDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(ctx, err, "read completed directory")
	}
	_, completedNumbers := scanPromptFiles(completedEntries)
	for n := range completedNumbers {
		usedNumbers[n] = true
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].name < files[j].name
	})

	return renameInvalidFiles(ctx, dir, files, usedNumbers, mover)
}

// scanPromptFiles scans directory entries and extracts file information.
func scanPromptFiles(entries []os.DirEntry) ([]fileInfo, map[int]bool) {
	files := make([]fileInfo, 0, len(entries))
	usedNumbers := make(map[int]bool)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		info := parseFilename(entry.Name(), validPatternRegexp, numericPatternRegexp)
		files = append(files, info)

		// Only claim the number if the file is already properly formatted (NNN-slug.md).
		// Wrong-format files (e.g. 01-foo.md) have a parsed number but haven't earned it yet.
		if info.number != -1 && validPatternRegexp.MatchString(entry.Name()) {
			usedNumbers[info.number] = true
		}
	}

	return files, usedNumbers
}

// parseFilename extracts number and slug from a filename.
func parseFilename(
	name string,
	validPattern *regexp.Regexp,
	numericPattern *regexp.Regexp,
) fileInfo {
	// Check if file matches valid pattern (3-digit)
	if matches := validPattern.FindStringSubmatch(name); matches != nil {
		num := 0
		_, _ = fmt.Sscanf(matches[1], "%d", &num)
		return fileInfo{name: name, number: num, slug: matches[2]}
	}

	// Check if file has any numeric prefix (wrong format or needs normalization)
	if matches := numericPattern.FindStringSubmatch(name); matches != nil {
		num := 0
		_, _ = fmt.Sscanf(matches[1], "%d", &num)
		return fileInfo{name: name, number: num, slug: matches[2]}
	}

	// No numeric prefix - assign -1 as placeholder
	slug := strings.TrimSuffix(name, ".md")
	return fileInfo{name: name, number: -1, slug: slug}
}

// renameInvalidFiles processes files and renames those that don't meet the naming convention.
func renameInvalidFiles(
	ctx context.Context,
	dir string,
	files []fileInfo,
	usedNumbers map[int]bool,
	mover FileMover,
) ([]Rename, error) {
	var renames []Rename
	seenNumbers := make(map[int]string)

	for _, f := range files {
		newNumber, needsRename := determineRename(f, seenNumbers, usedNumbers)

		if needsRename {
			rename, err := performRename(ctx, dir, f, newNumber, mover)
			if err != nil {
				return nil, err
			}
			renames = append(renames, rename)
			seenNumbers[newNumber] = rename.NewPath
		} else {
			seenNumbers[f.number] = f.name
		}
	}

	return renames, nil
}

// determineRename checks if a file needs to be renamed and returns the new number.
func determineRename(
	f fileInfo,
	seenNumbers map[int]string,
	usedNumbers map[int]bool,
) (int, bool) {
	// Case 1: No numeric prefix
	if f.number == -1 {
		newNum := findNextAvailableNumber(usedNumbers)
		usedNumbers[newNum] = true
		return newNum, true
	}

	// Case 2: Duplicate number
	if _, exists := seenNumbers[f.number]; exists {
		newNum := findNextAvailableNumber(usedNumbers)
		usedNumbers[newNum] = true
		return newNum, true
	}

	// Case 3: Wrong format
	expectedName := fmt.Sprintf("%03d-%s.md", f.number, f.slug)
	if f.name != expectedName {
		if usedNumbers[f.number] {
			newNum := findNextAvailableNumber(usedNumbers)
			usedNumbers[newNum] = true
			return newNum, true
		}
		usedNumbers[f.number] = true
		return f.number, true
	}

	return f.number, false
}

// findNextAvailableNumber finds the next unused number.
func findNextAvailableNumber(usedNumbers map[int]bool) int {
	for i := 1; ; i++ {
		if !usedNumbers[i] {
			return i
		}
	}
}

// performRename renames a file to match the naming convention.
func performRename(
	ctx context.Context,
	dir string,
	f fileInfo,
	newNumber int,
	mover FileMover,
) (Rename, error) {
	oldPath := filepath.Join(dir, f.name)
	newName := fmt.Sprintf("%03d-%s.md", newNumber, f.slug)
	newPath := filepath.Join(dir, newName)

	slog.Debug("normalizing filename", "from", f.name, "to", newName, "number", newNumber)

	if err := mover.MoveFile(ctx, oldPath, newPath); err != nil {
		return Rename{}, errors.Wrap(ctx, err, "rename file")
	}

	return Rename{OldPath: oldPath, NewPath: newPath}, nil
}

// readFrontmatter is a helper to read frontmatter from a file.
// Returns empty Frontmatter if file has no frontmatter delimiters.
func readFrontmatter(
	ctx context.Context,
	path string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (*Frontmatter, error) {
	pf, err := Load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "load prompt")
	}

	return &pf.Frontmatter, nil
}

// hasNumberPrefix checks if a filename has a numeric prefix (NNN-).
func hasNumberPrefix(filename string) bool {
	return hasNumberPrefixRegexp.MatchString(filename)
}

// extractNumberFromFilename extracts the numeric prefix from a filename.
// Returns -1 if the filename has no numeric prefix.
func extractNumberFromFilename(filename string) int {
	matches := extractNumberPrefixRegexp.FindStringSubmatch(filename)
	if matches == nil {
		return -1
	}
	num, err := strconv.Atoi(matches[1])
	if err != nil {
		return -1
	}
	return num
}

// AllPreviousCompleted checks if all prompts with numbers less than n are in completed directory.
func AllPreviousCompleted(ctx context.Context, completedDir string, n int) bool {
	if n <= 1 {
		return true // No previous prompts to check
	}

	completedEntries, err := os.ReadDir(completedDir)
	if err != nil {
		return false // completed directory doesn't exist or can't be read
	}

	// Collect all completed numbers
	completedNumbers := make(map[int]bool)
	for _, entry := range completedEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		num := extractNumberFromFilename(entry.Name())
		if num != -1 {
			completedNumbers[num] = true
		}
	}

	// Check that all numbers 1 through n-1 are completed
	for i := 1; i < n; i++ {
		if !completedNumbers[i] {
			return false
		}
	}

	return true
}

// FindMissingCompleted returns prompt numbers less than n that are NOT in the completed directory.
// Returns nil if all are completed.
func FindMissingCompleted(ctx context.Context, completedDir string, n int) []int {
	if n <= 1 {
		return nil
	}

	completedEntries, err := os.ReadDir(completedDir)
	if err != nil {
		// completed directory doesn't exist or can't be read — all are missing
		missing := make([]int, 0, n-1)
		for i := 1; i < n; i++ {
			missing = append(missing, i)
		}
		return missing
	}

	completedNumbers := make(map[int]bool)
	for _, entry := range completedEntries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		num := extractNumberFromFilename(entry.Name())
		if num != -1 {
			completedNumbers[num] = true
		}
	}

	var missing []int
	for i := 1; i < n; i++ {
		if !completedNumbers[i] {
			missing = append(missing, i)
		}
	}
	sort.Ints(missing)
	return missing
}

// FindPromptStatus looks up a prompt by number in the given directory and returns its frontmatter status.
// Returns empty string if not found.
func FindPromptStatus(ctx context.Context, dir string, number int) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	prefix := fmt.Sprintf("%03d-", number)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		// #nosec G304 -- path is constructed from os.ReadDir of a trusted directory
		data, err := os.ReadFile(path)
		if err != nil {
			return ""
		}
		var fm Frontmatter
		if _, err := frontmatter.Parse(bytes.NewReader(data), &fm); err != nil {
			return ""
		}
		return fm.Status
	}
	return ""
}
