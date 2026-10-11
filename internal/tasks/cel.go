package tasks

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
)

// evalCostLimit caps one guard's evaluation. The raw payload a guard sees
// is attacker-influenced and up to the webhook body cap, so the cost limit
// is what bounds a guard over it.
const evalCostLimit = 1_000_000

// envs are the CEL environments guards compile in: a task's own when
// sees the event; an action's also sees the answer.
var envs = sync.OnceValues(func() ([2]*cel.Env, error) {
	base := []cel.EnvOption{
		cel.Variable("event", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("subject", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("raw", cel.DynType),
	}
	trigger, err := cel.NewEnv(base...)
	if err != nil {
		return [2]*cel.Env{}, err
	}
	action, err := cel.NewEnv(append(base, cel.Variable("answer", cel.MapType(cel.StringType, cel.DynType)))...)
	return [2]*cel.Env{trigger, action}, err
})

// guard is a compiled when list: it holds when one of its conditions does.
type guard struct {
	conds []compiled
}

// compiled is one compiled condition.
type compiled struct {
	prg cel.Program
	src string
}

// compileGuard compiles each condition's expression in the trigger
// environment, or with answer in the action one. An empty list is nil,
// always true.
func compileGuard(when []When, withAnswer bool) (*guard, error) {
	if len(when) == 0 {
		return nil, nil
	}
	e, err := envs()
	if err != nil {
		return nil, fmt.Errorf("build CEL environment: %w", err)
	}
	env := e[0]
	if withAnswer {
		env = e[1]
	}
	g := &guard{}
	for i, w := range when {
		if w.Name != "" && slices.ContainsFunc(when[:i], func(o When) bool { return o.Name == w.Name }) {
			return nil, fmt.Errorf("when[%d]: name %q is given twice", i, w.Name)
		}
		if strings.TrimSpace(w.Expr) == "" {
			return nil, fmt.Errorf("when[%d]: expr is required", i)
		}
		ast, iss := env.Compile(w.Expr)
		if iss != nil && iss.Err() != nil {
			return nil, fmt.Errorf("when[%d] %q: %w", i, w.Expr, iss.Err())
		}
		switch ast.OutputType().Kind() {
		case types.BoolKind, types.DynKind:
		default:
			return nil, fmt.Errorf("when[%d] %q must evaluate to a boolean, got %s", i, w.Expr, ast.OutputType())
		}
		prg, err := env.Program(ast, cel.CostLimit(evalCostLimit))
		if err != nil {
			return nil, fmt.Errorf("when[%d] %q: %w", i, w.Expr, err)
		}
		g.conds = append(g.conds, compiled{prg: prg, src: w.Expr})
	}
	return g, nil
}

// eval reports whether the guard holds for vars: whether one of its
// conditions does. A nil guard always holds. A condition that fails to
// evaluate does not hold; when none holds, the first failure is returned
// with the expression that failed.
func (g *guard) eval(vars map[string]any) (ok bool, failed string, err error) {
	if g == nil {
		return true, "", nil
	}
	for _, c := range g.conds {
		out, _, cerr := c.prg.Eval(vars)
		if cerr == nil {
			b, isBool := out.Value().(bool)
			if isBool && b {
				return true, "", nil
			}
			if !isBool {
				cerr = fmt.Errorf("produced %T, want bool", out.Value())
			}
		}
		if cerr != nil && err == nil {
			failed, err = c.src, fmt.Errorf("tasks: eval %q: %w", c.src, cerr)
		}
	}
	return false, failed, err
}

// vars are the variables a guard sees of in, and of a when it is not nil.
func vars(in Input, a *Answer) map[string]any {
	v := map[string]any{
		"event": map[string]any{
			"forge": in.Forge, "name": in.Event, "rawEvent": in.RawEvent, "action": in.Action, "sender": in.Sender,
			"repo": map[string]any{"owner": in.Repo.Owner, "name": in.Repo.Name, "defaultBranch": in.Repo.DefaultBranch},
		},
		"subject": subjectVars(in.Subject),
		"raw":     rawOrEmpty(in.Raw),
	}
	if a != nil {
		v["answer"] = a.vars()
	}
	return v
}

// subjectVars is the subject variable; an absent subject has every key,
// with kind "", so a guard like subject.kind == "issue" is false rather
// than an error.
func subjectVars(s *Subject) map[string]any {
	if s == nil {
		s = &Subject{}
	}
	return map[string]any{
		"kind": s.Kind, "number": s.Number, "title": s.Title, keyBody: s.Body, keyState: s.State, "author": s.Author,
		"url": s.URL, keyLabels: orEmpty(s.Labels), keyAssignees: orEmpty(s.Assignees), "draft": s.Draft,
	}
}

func rawOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
