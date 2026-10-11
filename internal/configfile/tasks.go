package configfile

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/perfectra1n/kritique/internal/tasks"
)

// TaskBounds bound event tasks (see docs/adr), bound by bound: one written
// at a narrower scope replaces the broader scope's. A bound written nowhere
// takes its default; tasks are off unless enabled.
type TaskBounds struct {
	// Enabled turns tasks on, the operator's own included.
	Enabled *bool `yaml:"enabled,omitempty"`
	// Events are globs over the event names a repository's task may
	// trigger on, such as "issue.*" or "raw:release.*"; default the
	// normalized events, so a raw event must be listed. Only * and **
	// wildcards are accepted, since a trigger's own glob is matched
	// against them.
	Events []string `yaml:"events,omitempty"`
	// Actions are the action kinds a repository's task may declare;
	// default comment, labels and inlineComments, so state, assign and
	// reviewers must be listed.
	Actions []string `yaml:"actions,omitempty"`
	// Context are the context source kinds a repository's task may use;
	// default all but commands.
	Context []string `yaml:"context,omitempty"`
	// Tools are the agent tools a repository's task may use; default the
	// read-only read_file, grep and list_files.
	Tools []string `yaml:"tools,omitempty"`
	// SystemPrompt lets a repository's task add to the system prompt;
	// default false.
	SystemPrompt *bool `yaml:"systemPrompt,omitempty"`
	// Repository lets a repository's .kritique.yaml define tasks; default
	// true.
	Repository *bool `yaml:"repository,omitempty"`
	// MaxTasks caps a repository's tasks; default tasks.DefaultMaxTasks.
	MaxTasks *int `yaml:"maxTasks,omitempty"`
	// MaxRunsPerSubjectPerHour caps how often one task runs on one issue or
	// pull request; default tasks.DefaultMaxRunsPerSubjectPerHour.
	MaxRunsPerSubjectPerHour *int `yaml:"maxRunsPerSubjectPerHour,omitempty"`
	// MaxFields caps a repository task's fields; default
	// tasks.DefaultMaxFields.
	MaxFields *int `yaml:"maxFields,omitempty"`
}

// taskActionKinds and taskContextKinds are what the bounds may list.
var (
	taskActionKinds = []string{
		tasks.ActionComment, tasks.ActionLabels, tasks.ActionState, tasks.ActionAssign, tasks.ActionReviewers, tasks.ActionInlineComments,
	}
	taskContextKinds = []string{tasks.ContextThread, tasks.ContextFiles, tasks.ContextSearch, tasks.ContextRelated, tasks.ContextCommands}
)

// Resolve is the bounds with every default applied.
func (b TaskBounds) Resolve() tasks.Bounds {
	pick := func(v *int, def int) int {
		if v != nil {
			return *v
		}
		return def
	}
	orDefault := func(v, def []string) []string {
		if v == nil {
			return slices.Clone(def)
		}
		return slices.Clone(v)
	}
	return tasks.Bounds{
		Enabled: b.Enabled != nil && *b.Enabled,
		Events:  orDefault(b.Events, tasks.DefaultEvents), Actions: orDefault(b.Actions, tasks.DefaultActions),
		Context: orDefault(b.Context, tasks.DefaultContext), Tools: orDefault(b.Tools, tasks.DefaultTools),
		SystemPrompt: b.SystemPrompt != nil && *b.SystemPrompt, RepositoryTasks: b.Repository == nil || *b.Repository,
		MaxTasks:                 pick(b.MaxTasks, tasks.DefaultMaxTasks),
		MaxRunsPerSubjectPerHour: pick(b.MaxRunsPerSubjectPerHour, tasks.DefaultMaxRunsPerSubjectPerHour),
		MaxFields:                pick(b.MaxFields, tasks.DefaultMaxFields),
	}
}

// overlay lays the bounds o writes over b's.
func (b TaskBounds) overlay(o TaskBounds) TaskBounds {
	if o.Enabled != nil {
		b.Enabled = o.Enabled
	}
	if o.Events != nil {
		b.Events = o.Events
	}
	if o.Actions != nil {
		b.Actions = o.Actions
	}
	if o.Context != nil {
		b.Context = o.Context
	}
	if o.Tools != nil {
		b.Tools = o.Tools
	}
	if o.SystemPrompt != nil {
		b.SystemPrompt = o.SystemPrompt
	}
	if o.Repository != nil {
		b.Repository = o.Repository
	}
	if o.MaxTasks != nil {
		b.MaxTasks = o.MaxTasks
	}
	if o.MaxRunsPerSubjectPerHour != nil {
		b.MaxRunsPerSubjectPerHour = o.MaxRunsPerSubjectPerHour
	}
	if o.MaxFields != nil {
		b.MaxFields = o.MaxFields
	}
	return b
}

// validateTaskBounds checks one scope's task bounds name event globs and
// known kinds, and set positive caps.
func validateTaskBounds(where string, b TaskBounds) error {
	for i, g := range b.Events {
		if strings.TrimSpace(g) == "" || !doublestar.ValidatePattern(g) {
			return fmt.Errorf("configfile: %s.events[%d] %q is not a valid glob", where, i, g)
		}
		// A bound is matched against a trigger's own glob text, which
		// proves the trigger is inside it only for * and **: "?" or a
		// class also matches a trigger's "*".
		if strings.ContainsAny(g, "?[") {
			return fmt.Errorf("configfile: %s.events[%d] %q may use * and ** only", where, i, g)
		}
	}
	for _, c := range []struct {
		key   string
		got   []string
		known []string
	}{{"actions", b.Actions, taskActionKinds}, {"context", b.Context, taskContextKinds}} {
		for i, k := range c.got {
			if !slices.Contains(c.known, k) {
				return fmt.Errorf("configfile: %s.%s[%d] must be one of %s, got %q", where, c.key, i, strings.Join(c.known, ", "), k)
			}
		}
	}
	for i, tool := range b.Tools {
		if !toolRe.MatchString(tool) {
			return fmt.Errorf("configfile: %s.tools[%d] %q is not a tool name", where, i, tool)
		}
	}
	for _, c := range []struct {
		key string
		v   *int
	}{{"maxTasks", b.MaxTasks}, {"maxRunsPerSubjectPerHour", b.MaxRunsPerSubjectPerHour}, {"maxFields", b.MaxFields}} {
		if c.v != nil && *c.v <= 0 {
			return fmt.Errorf("configfile: %s.%s must be positive", where, c.key)
		}
	}
	return nil
}

// toolRe is an agent tool's name.
var toolRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// WithTasks is ts with more laid over them by name: one of more with a
// name ts has replaces that task where it stands, and the rest follow in
// order. A task switched off with enabled false still replaces one, so a
// narrower scope turns a broader scope's task off by naming it.
func WithTasks(ts, more []tasks.Task) []tasks.Task {
	if len(more) == 0 {
		return ts
	}
	out := slices.Clone(ts)
	for _, t := range more {
		if i := slices.IndexFunc(out, func(o tasks.Task) bool { return o.Name == t.Name }); i >= 0 {
			out[i] = t
		} else {
			out = append(out, t)
		}
	}
	return out
}

// enabledTasks is ts without those switched off.
func enabledTasks(ts []tasks.Task) []tasks.Task {
	return slices.DeleteFunc(slices.Clone(ts), func(t tasks.Task) bool { return !t.IsEnabled() })
}

// validateTasks checks each task one scope writes, that its models name
// providers declared for account a, and that no two share a name.
func (f *File) validateTasks(where string, a *Account, ts []tasks.Task) error {
	for i := range ts {
		at := fmt.Sprintf("%stasks[%d]", where, i)
		if err := ts[i].Check(); err != nil {
			return fmt.Errorf("configfile: %s: %w", at, err)
		}
		for _, m := range []struct{ key, ref string }{{"review", ts[i].Models.Review}, {"fallback", ts[i].Models.Fallback}} {
			if m.ref == "" {
				continue
			}
			if err := f.checkModelRef(at+".models."+m.key, a, ModelRef(m.ref)); err != nil {
				return err
			}
		}
		if slices.ContainsFunc(ts[:i], func(o tasks.Task) bool { return o.Name == ts[i].Name }) {
			return fmt.Errorf("configfile: %s: name %q is used twice", at, ts[i].Name)
		}
	}
	return nil
}
