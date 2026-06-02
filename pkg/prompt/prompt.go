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
	// CommittingPromptStatus indicates the container succeeded but the git commit is still pending.
	// The prompt stays in in-progress/ until the commit succeeds.
	CommittingPromptStatus PromptStatus = "committing"
	// RejectedPromptStatus indicates the prompt was deliberately abandoned before execution.
	// This is a terminal state — rejected prompts are moved to prompts/rejected/ and never executed.
	RejectedPromptStatus PromptStatus = "rejected"
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
	CommittingPromptStatus,
	RejectedPromptStatus,
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

// promptTransitions defines the valid state transitions for prompt lifecycle.
// This is the single source of truth — add one row here to enable a new transition.
var promptTransitions = map[PromptStatus][]PromptStatus{
	IdeaPromptStatus:  {DraftPromptStatus, RejectedPromptStatus},
	DraftPromptStatus: {ApprovedPromptStatus, RejectedPromptStatus},
	ApprovedPromptStatus: {
		ExecutingPromptStatus,
		CancelledPromptStatus,
		DraftPromptStatus,
		RejectedPromptStatus,
	}, // unapprove: approved → draft
	ExecutingPromptStatus: {
		CommittingPromptStatus,
		FailedPromptStatus,
		CancelledPromptStatus,
	},
	CommittingPromptStatus:          {CompletedPromptStatus, FailedPromptStatus},
	FailedPromptStatus:              {ApprovedPromptStatus, CancelledPromptStatus},
	InReviewPromptStatus:            {PendingVerificationPromptStatus, FailedPromptStatus},
	PendingVerificationPromptStatus: {CompletedPromptStatus, FailedPromptStatus},
}

// CanTransitionTo returns nil if transitioning from s to target is valid,
// or an error naming both states if the transition is not in the table.
func (s PromptStatus) CanTransitionTo(ctx context.Context, target PromptStatus) error {
	for _, allowed := range promptTransitions[s] {
		if allowed == target {
			return nil
		}
	}
	return errors.Errorf(ctx, "cannot transition prompt from %q to %q", s, target)
}

// IsTerminal returns true if the prompt has reached a final, non-actionable state.
func (s PromptStatus) IsTerminal() bool {
	return s == CompletedPromptStatus || s == CancelledPromptStatus
}

// IsPreExecution returns true if the prompt has not yet entered active execution.
func (s PromptStatus) IsPreExecution() bool {
	return s == IdeaPromptStatus || s == DraftPromptStatus || s == ApprovedPromptStatus
}

// IsActive returns true if the prompt is in active processing (neither pre-execution nor terminal).
// Note: FailedPromptStatus is intentionally Active — failed prompts can be re-approved for retry.
func (s PromptStatus) IsActive() bool {
	return !s.IsPreExecution() && !s.IsTerminal()
}

// IsRejectable returns true if the prompt may be rejected from its current state.
// Rejection is only allowed from pre-execution states (idea, draft, approved).
func (s PromptStatus) IsRejectable() bool {
	return s.IsPreExecution()
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
		return errors.Errorf(
			context.Background(),
			"unexpected YAML node kind for spec: %v",
			value.Kind,
		)
	}
}

// Frontmatter represents the YAML frontmatter in a prompt file.
type Frontmatter struct {
	Status             string   `yaml:"status"`
	OriginalStatus     string   `yaml:"originalStatus,omitempty"`
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
	Rejected           string   `yaml:"rejected,omitempty"`
	RejectedReason     string   `yaml:"rejected_reason,omitempty"`
	Cancelled          string   `yaml:"cancelled,omitempty"`
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

// load reads a prompt file from disk, parsing frontmatter and body.
// Body is stored as-is and never modified by Save.
func load(
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

// MarkCompleted sets status to completed with timestamp and clears any
// previously recorded lastFailReason so a successful retry leaves no stale
// failure data in the frontmatter. The YAML tag is lastFailReason,omitempty
// so the field is dropped entirely from the serialised file when empty.
func (pf *PromptFile) MarkCompleted() {
	pf.Frontmatter.Status = string(CompletedPromptStatus)
	pf.Frontmatter.Completed = pf.now().UTC().Format(time.RFC3339)
	pf.Frontmatter.LastFailReason = ""
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

// StampRejected sets the rejected timestamp and reason, then marks status as rejected.
func (pf *PromptFile) StampRejected(reason string) {
	if pf.Frontmatter.Rejected == "" {
		pf.Frontmatter.Rejected = pf.now().UTC().Format(time.RFC3339)
	}
	pf.Frontmatter.RejectedReason = reason
	pf.Frontmatter.Status = string(RejectedPromptStatus)
}

// StampRejectedWithOriginal sets the rejected timestamp and reason, marks status as rejected,
// and preserves the prompt's prior status (typically "failed") in the originalStatus field.
// Used by the reject command when rejecting a prompt from a non-pre-execution state.
func (pf *PromptFile) StampRejectedWithOriginal(reason, originalStatus string) {
	if pf.Frontmatter.Rejected == "" {
		pf.Frontmatter.Rejected = pf.now().UTC().Format(time.RFC3339)
	}
	pf.Frontmatter.RejectedReason = reason
	pf.Frontmatter.Status = string(RejectedPromptStatus)
	pf.Frontmatter.OriginalStatus = originalStatus
}

// MarkPendingVerification sets status to pending_verification.
func (pf *PromptFile) MarkPendingVerification() {
	pf.Frontmatter.Status = string(PendingVerificationPromptStatus)
}

// MarkCancelled sets status to cancelled with a UTC timestamp.
func (pf *PromptFile) MarkCancelled() {
	pf.Frontmatter.Cancelled = pf.now().UTC().Format(time.RFC3339)
	pf.Frontmatter.Status = string(CancelledPromptStatus)
}

// MarkCommitting sets the status to "committing" — container succeeded, awaiting git commit.
func (pf *PromptFile) MarkCommitting() {
	pf.Frontmatter.Status = string(CommittingPromptStatus)
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

// Summary extracts the content of the <summary> tag from the prompt body.
// Returns an empty string if no <summary> tag is found.
func (pf *PromptFile) Summary() string {
	body := string(pf.Body)
	const openTag = "<summary>"
	const closeTag = "</summary>"
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

// NewManager creates a new Manager.
func NewManager(
	inboxDir string,
	inProgressDir string,
	completedDir string,
	cancelledDir string,
	mover FileMover,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) *Manager {
	m := &Manager{
		inboxDir:              inboxDir,
		inProgressDir:         inProgressDir,
		completedDir:          completedDir,
		cancelledDir:          cancelledDir,
		mover:                 mover,
		currentDateTimeGetter: currentDateTimeGetter,
	}
	m.promptStatusManager = NewPromptStatusManager(currentDateTimeGetter)
	m.promptScanner = NewPromptScanner(inProgressDir, completedDir, currentDateTimeGetter)
	m.promptMover = NewPromptMover(
		inProgressDir,
		completedDir,
		cancelledDir,
		mover,
		currentDateTimeGetter,
	)
	m.promptFileLoader = NewPromptFileLoader(currentDateTimeGetter)
	return m
}

// Manager manages prompt file operations.
type Manager struct {
	inboxDir              string
	inProgressDir         string
	completedDir          string
	cancelledDir          string
	mover                 FileMover
	currentDateTimeGetter libtime.CurrentDateTimeGetter

	promptStatusManager PromptStatusManager
	promptScanner       PromptScanner
	promptMover         PromptMover
	promptFileLoader    PromptFileLoader
}

// PromptStatusManager handles all status mutations for prompt files.
type PromptStatusManager struct {
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewPromptStatusManager creates a PromptStatusManager.
func NewPromptStatusManager(
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) PromptStatusManager {
	return PromptStatusManager{
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// SetStatus updates the status field in a prompt file's frontmatter.
func (p PromptStatusManager) SetStatus(ctx context.Context, path string, status string) error {
	return setStatus(ctx, path, status, p.currentDateTimeGetter)
}

// SetContainer updates the container field in a prompt file's frontmatter.
func (p PromptStatusManager) SetContainer(ctx context.Context, path string, name string) error {
	return setContainer(ctx, path, name, p.currentDateTimeGetter)
}

// SetVersion updates the dark-factory-version field in a prompt file's frontmatter.
func (p PromptStatusManager) SetVersion(ctx context.Context, path string, version string) error {
	return setVersion(ctx, path, version, p.currentDateTimeGetter)
}

// SetPRURL updates the pr-url field in a prompt file's frontmatter.
func (p PromptStatusManager) SetPRURL(ctx context.Context, path string, url string) error {
	return setPRURL(ctx, path, url, p.currentDateTimeGetter)
}

// SetBranch updates the branch field in a prompt file's frontmatter.
func (p PromptStatusManager) SetBranch(ctx context.Context, path string, branch string) error {
	return setBranch(ctx, path, branch, p.currentDateTimeGetter)
}

// IncrementRetryCount increments the retryCount field in a prompt file's frontmatter.
func (p PromptStatusManager) IncrementRetryCount(ctx context.Context, path string) error {
	return incrementRetryCount(ctx, path, p.currentDateTimeGetter)
}

// PromptScanner handles directory queries for prompt files.
type PromptScanner struct {
	inProgressDir         string
	completedDir          string
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewPromptScanner creates a PromptScanner.
func NewPromptScanner(
	inProgressDir, completedDir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) PromptScanner {
	return PromptScanner{
		inProgressDir:         inProgressDir,
		completedDir:          completedDir,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// ListQueued scans the in-progress directory for .md files ready to be picked up.
func (p PromptScanner) ListQueued(ctx context.Context) ([]Prompt, error) {
	return listQueued(ctx, p.inProgressDir, p.currentDateTimeGetter)
}

// HasExecuting returns true if any prompt in the directory has status "executing".
func (p PromptScanner) HasExecuting(ctx context.Context) bool {
	return hasExecuting(ctx, p.inProgressDir, p.currentDateTimeGetter)
}

// FindCommitting returns paths of all prompt files in in-progress/ with status "committing".
func (p PromptScanner) FindCommitting(ctx context.Context) ([]string, error) {
	return findCommitting(ctx, p.inProgressDir, p.currentDateTimeGetter)
}

// FindPromptStatusInProgress looks up a prompt by number in the in-progress directory and returns its status.
func (p PromptScanner) FindPromptStatusInProgress(ctx context.Context, number int) string {
	return findPromptStatus(ctx, p.inProgressDir, number)
}

// AllPreviousCompleted checks if all prompts with numbers less than n are in completed/.
func (p PromptScanner) AllPreviousCompleted(ctx context.Context, n int) bool {
	return allPreviousCompleted(ctx, p.completedDir, n)
}

// FindMissingCompleted returns prompt numbers less than n that are NOT in completed/.
func (p PromptScanner) FindMissingCompleted(ctx context.Context, n int) []int {
	return findMissingCompleted(ctx, p.completedDir, n)
}

// AllPreviousInSpecCompleted checks if the predecessor prompt within the same spec
// is in the completed directory. Specifically: walks in-progress/ AND completed/
// for files whose spec field includes specID and whose number is strictly less
// than n; the highest such number M is the predecessor; returns true iff M is in
// completed/.
//
// If no predecessor is found (candidate is the first prompt of its spec),
// returns true (no predecessor to check). If specID is empty, returns true
// (caller should fall back to global guard at the scanner layer).
func (p PromptScanner) AllPreviousInSpecCompleted(ctx context.Context, n int, specID string) bool {
	return allPreviousInSpecCompleted(
		ctx,
		p.completedDir,
		p.inProgressDir,
		n,
		specID,
		p.currentDateTimeGetter,
	)
}

// FindMissingInSpecCompleted returns the number of the predecessor prompt
// within the same spec that is NOT in completed/, or -1 if no predecessor
// exists for the candidate. Walks in-progress/ AND completed/.
func (p PromptScanner) FindMissingInSpecCompleted(
	ctx context.Context,
	n int,
	specID string,
) (int, error) {
	return findMissingInSpecCompleted(
		ctx,
		p.completedDir,
		p.inProgressDir,
		n,
		specID,
		p.currentDateTimeGetter,
	)
}

// PromptMover handles file movement operations for prompt files.
type PromptMover struct {
	inProgressDir         string
	completedDir          string
	cancelledDir          string
	mover                 FileMover
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewPromptMover creates a PromptMover.
func NewPromptMover(
	inProgressDir string,
	completedDir string,
	cancelledDir string,
	mover FileMover,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) PromptMover {
	return PromptMover{
		inProgressDir:         inProgressDir,
		completedDir:          completedDir,
		cancelledDir:          cancelledDir,
		mover:                 mover,
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// MoveToCompleted sets status to "completed" and moves a prompt file to the completed directory.
func (p PromptMover) MoveToCompleted(ctx context.Context, path string) error {
	return moveToCompleted(ctx, path, p.completedDir, p.mover, p.currentDateTimeGetter)
}

// MoveToCancelled sets status to "cancelled" (with timestamp) and moves a prompt file to the cancelled directory.
func (p PromptMover) MoveToCancelled(ctx context.Context, path string) error {
	return moveToCancelled(ctx, path, p.cancelledDir, p.mover, p.currentDateTimeGetter)
}

// NormalizeFilenames scans a directory for .md files and ensures they follow the NNN-slug.md naming convention.
func (p PromptMover) NormalizeFilenames(ctx context.Context, dir string) ([]Rename, error) {
	return normalizeFilenames(ctx, dir, p.completedDir, p.mover)
}

// PrepareRollback prepares a prompt file for rollback: loads it, sets status to CommittingPromptStatus, and saves.
// This separates state preparation from I/O so that RollbackMove can be retried independently.
func (p PromptMover) PrepareRollback(ctx context.Context, completedPath string) error {
	pf, err := load(ctx, completedPath, p.currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt for rollback")
	}
	pf.Frontmatter.Status = string(CommittingPromptStatus)
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "save frontmatter for rollback")
	}
	return nil
}

// RollbackMove moves a prompt file from the completed directory back to the in-progress directory.
// The file must already have been prepared via PrepareRollback.
func (p PromptMover) RollbackMove(ctx context.Context, completedPath string) error {
	originalPath := filepath.Join(p.inProgressDir, filepath.Base(completedPath))
	if err := p.mover.MoveFile(ctx, completedPath, originalPath); err != nil {
		return errors.Wrap(ctx, err, "rollback move to completed")
	}
	slog.InfoContext(
		ctx,
		"move-rolled-back-after-commit-failure",
		"file",
		filepath.Base(completedPath),
	)
	return nil
}

// PromptFileLoader handles file I/O for prompt files.
type PromptFileLoader struct {
	currentDateTimeGetter libtime.CurrentDateTimeGetter
}

// NewPromptFileLoader creates a PromptFileLoader.
func NewPromptFileLoader(currentDateTimeGetter libtime.CurrentDateTimeGetter) PromptFileLoader {
	return PromptFileLoader{
		currentDateTimeGetter: currentDateTimeGetter,
	}
}

// Load reads a prompt file from disk, parsing frontmatter and body.
func (p PromptFileLoader) Load(ctx context.Context, path string) (*PromptFile, error) {
	return load(ctx, path, p.currentDateTimeGetter)
}

// Content returns the prompt content (without frontmatter) for passing to Docker.
func (p PromptFileLoader) Content(ctx context.Context, path string) (string, error) {
	return content(ctx, path, p.currentDateTimeGetter)
}

// Title extracts the first # heading from a prompt file.
func (p PromptFileLoader) Title(ctx context.Context, path string) (string, error) {
	return title(ctx, path, p.currentDateTimeGetter)
}

// ReadFrontmatter reads frontmatter from a file.
func (p PromptFileLoader) ReadFrontmatter(ctx context.Context, path string) (*Frontmatter, error) {
	return readFrontmatter(ctx, path, p.currentDateTimeGetter)
}

// ResetExecuting resets any prompts with status "executing" back to "approved".
func (pm *Manager) ResetExecuting(ctx context.Context) error {
	return resetExecuting(ctx, pm.inProgressDir, pm.currentDateTimeGetter)
}

// ResetFailed resets any prompts with status "failed" back to "approved".
func (pm *Manager) ResetFailed(ctx context.Context) error {
	return resetFailed(ctx, pm.inProgressDir, pm.currentDateTimeGetter)
}

// HasExecuting returns true if any prompt in dir has status "executing".
func (pm *Manager) HasExecuting(ctx context.Context) bool {
	return pm.promptScanner.HasExecuting(ctx)
}

// ListQueued scans a directory for .md files that should be picked up.
func (pm *Manager) ListQueued(ctx context.Context) ([]Prompt, error) {
	return pm.promptScanner.ListQueued(ctx)
}

// FindCommitting returns paths of all prompt files in in-progress/ with status "committing".
func (pm *Manager) FindCommitting(ctx context.Context) ([]string, error) {
	return pm.promptScanner.FindCommitting(ctx)
}

// Load reads a prompt file from disk, parsing frontmatter and body.
func (pm *Manager) Load(ctx context.Context, path string) (*PromptFile, error) {
	return pm.promptFileLoader.Load(ctx, path)
}

// ReadFrontmatter reads frontmatter from a file.
func (pm *Manager) ReadFrontmatter(ctx context.Context, path string) (*Frontmatter, error) {
	return pm.promptFileLoader.ReadFrontmatter(ctx, path)
}

// SetStatus updates the status field in a prompt file's frontmatter.
func (pm *Manager) SetStatus(ctx context.Context, path string, status string) error {
	return pm.promptStatusManager.SetStatus(ctx, path, status)
}

// SetContainer updates the container field in a prompt file's frontmatter.
func (pm *Manager) SetContainer(ctx context.Context, path string, name string) error {
	return pm.promptStatusManager.SetContainer(ctx, path, name)
}

// SetVersion updates the dark-factory-version field in a prompt file's frontmatter.
func (pm *Manager) SetVersion(ctx context.Context, path string, version string) error {
	return pm.promptStatusManager.SetVersion(ctx, path, version)
}

// SetPRURL updates the pr-url field in a prompt file's frontmatter.
func (pm *Manager) SetPRURL(ctx context.Context, path string, url string) error {
	return pm.promptStatusManager.SetPRURL(ctx, path, url)
}

// SetBranch updates the branch field in a prompt file's frontmatter.
func (pm *Manager) SetBranch(ctx context.Context, path string, branch string) error {
	return pm.promptStatusManager.SetBranch(ctx, path, branch)
}

// IncrementRetryCount increments the retryCount field in a prompt file's frontmatter.
func (pm *Manager) IncrementRetryCount(ctx context.Context, path string) error {
	return pm.promptStatusManager.IncrementRetryCount(ctx, path)
}

// Content returns the prompt content (without frontmatter) for passing to Docker.
func (pm *Manager) Content(ctx context.Context, path string) (string, error) {
	return pm.promptFileLoader.Content(ctx, path)
}

// Title extracts the first # heading from a prompt file.
func (pm *Manager) Title(ctx context.Context, path string) (string, error) {
	return pm.promptFileLoader.Title(ctx, path)
}

// MoveToCompleted sets status to "completed" and moves a prompt file to the completed/ subdirectory.
func (pm *Manager) MoveToCompleted(ctx context.Context, path string) error {
	return pm.promptMover.MoveToCompleted(ctx, path)
}

// MoveToCancelled sets status to "cancelled" (with timestamp) and moves a prompt file to the cancelled/ subdirectory.
func (pm *Manager) MoveToCancelled(ctx context.Context, path string) error {
	return pm.promptMover.MoveToCancelled(ctx, path)
}

// NormalizeFilenames scans a directory for .md files and ensures they follow the NNN-slug.md naming convention.
// It also checks the completed directory for used numbers.
func (pm *Manager) NormalizeFilenames(ctx context.Context, dir string) ([]Rename, error) {
	return pm.promptMover.NormalizeFilenames(ctx, dir)
}

// AllPreviousCompleted checks if all prompts with numbers less than n are in completed/.
func (pm *Manager) AllPreviousCompleted(ctx context.Context, n int) bool {
	return pm.promptScanner.AllPreviousCompleted(ctx, n)
}

// FindMissingCompleted returns prompt numbers less than n that are NOT in completed/.
func (pm *Manager) FindMissingCompleted(ctx context.Context, n int) []int {
	return pm.promptScanner.FindMissingCompleted(ctx, n)
}

// FindPromptStatusInProgress looks up a prompt by number in the in-progress directory and returns its frontmatter status.
func (pm *Manager) FindPromptStatusInProgress(ctx context.Context, number int) string {
	return pm.promptScanner.FindPromptStatusInProgress(ctx, number)
}

// AllPreviousInSpecCompleted checks if the predecessor prompt in the same spec is completed.
func (pm *Manager) AllPreviousInSpecCompleted(ctx context.Context, n int, specID string) bool {
	return pm.promptScanner.AllPreviousInSpecCompleted(ctx, n, specID)
}

// FindMissingInSpecCompleted returns the predecessor number in the same spec that is NOT completed.
func (pm *Manager) FindMissingInSpecCompleted(
	ctx context.Context,
	n int,
	specID string,
) (int, error) {
	return pm.promptScanner.FindMissingInSpecCompleted(ctx, n, specID)
}

// HasQueuedPromptsOnBranch returns true if any queued prompt (other than excludePath)
// has the given branch value in its frontmatter.
func (pm *Manager) HasQueuedPromptsOnBranch(
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
func listQueued(
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
			fm.Status == string(CommittingPromptStatus) ||
			fm.Status == string(CompletedPromptStatus) ||
			fm.Status == string(FailedPromptStatus) ||
			fm.Status == string(InReviewPromptStatus) ||
			fm.Status == string(PendingVerificationPromptStatus) ||
			fm.Status == string(CancelledPromptStatus) {
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
func resetExecuting(
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
		pf, err := load(ctx, path, currentDateTimeGetter)
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
func resetFailed(
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
		pf, err := load(ctx, path, currentDateTimeGetter)
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

// FindCommitting returns the paths of all .md files in dir whose status is "committing".
// Files that cannot be read are skipped with a warning.
func findCommitting(
	ctx context.Context,
	dir string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errors.Wrap(ctx, err, "read directory")
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		fm, err := readFrontmatter(ctx, path, currentDateTimeGetter)
		if err != nil {
			slog.Warn("skipping prompt in FindCommitting", "file", entry.Name(), "error", err)
			continue
		}
		if fm.Status == string(CommittingPromptStatus) {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// SetStatus updates the status field in a prompt file's frontmatter.
// If the file has no frontmatter, adds frontmatter with the status field.
// Also sets appropriate timestamp fields based on status.
func setStatus(
	ctx context.Context,
	path string,
	status string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	pf, err := load(ctx, path, currentDateTimeGetter)
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
func setContainer(
	ctx context.Context,
	path string,
	container string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	pf, err := load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.Frontmatter.Container = container
	return pf.Save(ctx)
}

// SetVersion updates the dark-factory-version field in a prompt file's frontmatter.
// If the file has no frontmatter, adds frontmatter with the version field.
func setVersion(
	ctx context.Context,
	path string,
	version string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	pf, err := load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.Frontmatter.DarkFactoryVersion = version
	return pf.Save(ctx)
}

// SetPRURL updates the pr-url field in a prompt file's frontmatter.
// If the file has no frontmatter, adds frontmatter with the pr-url field.
func setPRURL(
	ctx context.Context,
	path string,
	url string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	pf, err := load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.SetPRURL(url)
	return pf.Save(ctx)
}

// SetBranch updates the branch field in a prompt file's frontmatter.
// If the file has no frontmatter, adds frontmatter with the branch field.
func setBranch(
	ctx context.Context,
	path string,
	branch string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	pf, err := load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.SetBranch(branch)
	return pf.Save(ctx)
}

// IncrementRetryCount increments the retryCount field in a prompt file's frontmatter by 1.
// If the file has no frontmatter, adds frontmatter with retryCount set to 1.
func incrementRetryCount(
	ctx context.Context,
	path string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	pf, err := load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.Frontmatter.RetryCount++
	return pf.Save(ctx)
}

// Title extracts the first # heading from a prompt file.
// Handles files with or without frontmatter.
// If no heading is found, returns the filename without extension.
func title(
	ctx context.Context,
	path string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (string, error) {
	pf, err := load(ctx, path, currentDateTimeGetter)
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
func content(
	ctx context.Context,
	path string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (string, error) {
	pf, err := load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return "", errors.Wrap(ctx, err, "load prompt")
	}

	return pf.Content()
}

// RollbackMoveToCompleted is the inverse of MoveToCompleted.
// It moves a prompt file from pm.completedDir back to pm.inProgressDir
// and restores its frontmatter status to CommittingPromptStatus
// (the state the prompt was in immediately before MoveToCompleted ran).
// Used by workflow executors when the work commit fails after the move.
func (pm *Manager) RollbackMoveToCompleted(
	ctx context.Context,
	completedPath string,
	mover FileMover,
) error {
	// PrepareRollback: load, set status to Committing, save (I/O deferred)
	if err := pm.promptMover.PrepareRollback(ctx, completedPath); err != nil {
		return errors.Wrap(ctx, err, "prepare rollback")
	}

	// RollbackMove: actual file I/O
	originalPath := filepath.Join(pm.inProgressDir, filepath.Base(completedPath))
	if err := mover.MoveFile(ctx, completedPath, originalPath); err != nil {
		return errors.Wrap(ctx, err, "rollback move to completed")
	}

	slog.InfoContext(
		ctx,
		"move-rolled-back-after-commit-failure",
		"file",
		filepath.Base(completedPath),
	)
	return nil
}

// MoveToCompleted sets status to "completed" and moves a prompt file to the completed directory.
// This ensures files in completed/ always have the correct status.
func moveToCompleted(
	ctx context.Context,
	path string,
	completedDir string,
	mover FileMover,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	// Load, mark completed, and save before moving
	pf, err := load(ctx, path, currentDateTimeGetter)
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

// moveToCancelled sets status to "cancelled" (with timestamp) and moves a prompt file to the cancelled directory.
func moveToCancelled(
	ctx context.Context,
	path string,
	cancelledDir string,
	mover FileMover,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) error {
	pf, err := load(ctx, path, currentDateTimeGetter)
	if err != nil {
		return errors.Wrap(ctx, err, "load prompt")
	}

	pf.MarkCancelled()
	if err := pf.Save(ctx); err != nil {
		return errors.Wrap(ctx, err, "set cancelled status")
	}

	if err := os.MkdirAll(cancelledDir, 0750); err != nil {
		return errors.Wrap(ctx, err, "create cancelled directory")
	}

	filename := filepath.Base(path)
	dest := filepath.Join(cancelledDir, filename)

	slog.Debug("moving to cancelled", "from", path, "to", dest)

	if err := mover.MoveFile(ctx, path, dest); err != nil {
		return errors.Wrap(ctx, err, "move file")
	}

	return nil
}

// HasExecuting returns true if any prompt in dir has status "executing".
func hasExecuting(
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

// readFrontmatter is a helper to read frontmatter from a file.
// Returns empty Frontmatter if file has no frontmatter delimiters.
func readFrontmatter(
	ctx context.Context,
	path string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (*Frontmatter, error) {
	pf, err := load(ctx, path, currentDateTimeGetter)
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
func allPreviousCompleted(_ context.Context, completedDir string, n int) bool {
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
func findMissingCompleted(_ context.Context, completedDir string, n int) []int {
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

// allPreviousInSpecCompleted checks if the predecessor prompt in the same spec
// is in the completed directory. The predecessor is the largest declared number
// strictly less than n in the same spec; that number must exist in completed/.
// A gap (e.g. 224 missing between 223 and 225) is detected: the predecessor is
// 224 (the undeclared number just below n), and "missing from completed" blocks
// the candidate. If no predecessor exists at all, returns true. If specID is
// empty, returns true (caller should fall back to global guard at the scanner
// layer).
func allPreviousInSpecCompleted(
	_ context.Context,
	completedDir string,
	scanDir string,
	n int,
	specID string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) bool {
	if specID == "" {
		return true
	}
	pred, ok := findPredecessorInSpec(scanDir, completedDir, n, specID, currentDateTimeGetter)
	if !ok {
		return true
	}
	return isNumberInCompletedDir(completedDir, pred)
}

// findMissingInSpecCompleted returns the predecessor number in the same spec
// that is NOT in completed/, or -1 if no predecessor exists.
func findMissingInSpecCompleted(
	_ context.Context,
	completedDir string,
	scanDir string,
	n int,
	specID string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (int, error) {
	if specID == "" {
		return -1, nil
	}
	pred, ok := findPredecessorInSpec(scanDir, completedDir, n, specID, currentDateTimeGetter)
	if !ok {
		return -1, nil
	}
	if isNumberInCompletedDir(completedDir, pred) {
		return -1, nil
	}
	return pred, nil
}

// findPredecessorInSpec walks both scanDir (in-progress/) and completedDir for
// prompt files whose spec field includes specID and whose number is strictly
// less than n. Returns the highest such number and true, or (-1, false) if no
// such prompt exists.
//
// The returned number is the largest declared prompt for this spec below n. If
// the declared largest is < n-1, the implicit predecessor is n-1 (a gap), and
// the caller reports that number as missing. This matches the "missing
// predecessor" test contract: 225 in in-progress, 223 in completed, 224 absent
// → predecessor is 224 (the undeclared n-1), and is reported missing.
//
//nolint:gocognit // two-dir scan + gap detection + highest-below-n tracking; refactor candidate tracked separately
func findPredecessorInSpec(
	scanDir string,
	completedDir string,
	n int,
	specID string,
	currentDateTimeGetter libtime.CurrentDateTimeGetter,
) (int, bool) {
	highest := -1
	for _, dir := range []string{scanDir, completedDir} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			num := extractNumberFromFilename(entry.Name())
			if num < 0 || num >= n {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			fm, err := readFrontmatter(context.Background(), path, currentDateTimeGetter)
			if err != nil {
				continue
			}
			if !specListContains(fm.Specs, specID) {
				continue
			}
			if num > highest {
				highest = num
			}
		}
	}
	// If the largest declared is less than n-1, the immediate predecessor
	// (n-1) is undeclared — treat it as the predecessor and report it as
	// missing. This implements "gap detection": e.g. 223 in completed, 225
	// queued, 224 absent → predecessor is 224, not in completed → block.
	if highest >= 0 && highest < n-1 {
		return n - 1, true
	}
	return highest, highest >= 0
}

// isNumberInCompletedDir returns true if a file with the given number exists in completedDir.
func isNumberInCompletedDir(completedDir string, num int) bool {
	entries, err := os.ReadDir(completedDir)
	if err != nil {
		return false
	}
	prefix := fmt.Sprintf("%03d-", num)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if strings.HasPrefix(entry.Name(), prefix) {
			return true
		}
	}
	return false
}

// specListContains returns true if specID matches any entry in the spec list.
// Comparison uses specnum.Parse to normalize numeric prefixes ("058" == "058-foo-bar" == 58).
// When specnum.Parse returns -1 for both sides (no numeric prefix), falls back to string equality.
func specListContains(specs SpecList, specID string) bool {
	target := specnum.Parse(specID)
	for _, s := range specs {
		if target >= 0 {
			if specnum.Parse(s) == target {
				return true
			}
		} else {
			if s == specID {
				return true
			}
		}
	}
	return false
}

// FindPromptStatus looks up a prompt by number in the given directory and returns its frontmatter status.
// Returns empty string if not found.
func findPromptStatus(_ context.Context, dir string, number int) string {
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
