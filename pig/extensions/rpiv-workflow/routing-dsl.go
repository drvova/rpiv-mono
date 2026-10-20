package workflow

import (
	"regexp"
)

// The routing DSL — edge vocabulary (EdgeFn / EdgeTarget / Stop) and the
// route builders (DefineRoute, Gate, Match) with their structural markers
// (ReadsData, RouteNote). Runtime edge EXECUTION lives in the audit-side
// routing package — this file is authoring-surface only.

// EdgeContext is the runtime context handed to an EdgeFn.
type EdgeContext struct {
	Output *Output
	State  RunView
}

// EdgePredicate is the body of a hand-rolled route pick. Users wrap it via
// DefineRoute, which returns an EdgeFn (this plus Targets).
type EdgePredicate func(ctx EdgeContext) (string, error)

// EdgeFn picks the next stage name given current state + output. Targets
// lets graph introspectors enumerate possible returns — Gate / Match and
// DefineRoute populate it. It carries SymbolMarks (ReadsData / RouteNote).
type EdgeFn struct {
	SymbolMarks
	fn      EdgePredicate
	Targets []string
}

// Pick runs the route against ctx.
func (e *EdgeFn) Pick(ctx EdgeContext) (string, error) { return e.fn(ctx) }

// Stop is the terminal edge sentinel — the single source of truth for the
// "stop" literal embedded in EdgeTarget.
const Stop = "stop"

// EdgeTarget is what an Edges entry resolves to: another stage name
// (string), the terminal sentinel Stop (string), or an *EdgeFn chosen at
// run time. Build with To / Route.
type EdgeTarget struct {
	name string
	fn   *EdgeFn
}

// To builds a linear edge to a stage name (or Stop).
func To(name string) EdgeTarget { return EdgeTarget{name: name} }

// Route builds a run-time-decided edge.
func Route(fn *EdgeFn) EdgeTarget { return EdgeTarget{fn: fn} }

// IsZero reports whether the target is unset (no edge declared).
func (t EdgeTarget) IsZero() bool { return t.fn == nil && t.name == "" }

// Name is the stage name of a linear edge ("" for a route).
func (t EdgeTarget) Name() string { return t.name }

// Fn is the route of a run-time edge (nil for a linear one).
func (t EdgeTarget) Fn() *EdgeFn { return t.fn }

// IsStop reports whether the target is the terminal sentinel.
func (t EdgeTarget) IsStop() bool { return t.fn == nil && t.name == Stop }

// ReadsData is the marker attached to EdgeFns that read from output.data.
// The validator warns when a stage feeds a marked route but has no
// OutputSchema — routing on un-validated data is a defect. DefineRoute
// auto-marks; opt out with DefineRouteOptions{ReadsData: Bool(false)}.
var ReadsData = SymbolFor("rpiv.workflow.readsData")

// MarksReadsData is true iff fn was wrapped by DefineRoute with the
// data-reading marker attached.
func MarksReadsData(fn *EdgeFn) bool {
	if fn == nil {
		return false
	}
	v, ok := ReadSymbol[bool](fn, ReadsData)
	return ok && v
}

// DefineRouteOptions are the options for DefineRoute. ReadsData nil/true
// marks the route data-reading; false suppresses the OutputSchema lint for
// routes that consult only state / output.meta.
type DefineRouteOptions struct {
	ReadsData *bool
}

// DefineRoute promotes a hand-rolled EdgePredicate to an EdgeFn by
// structurally attaching the set of possible returns. The validator requires
// every EdgeFn to carry Targets so reachability and edge-target checks see
// every branch; this is the only blessed way to author a multi-branch route.
// A fresh wrapper is built every call — the caller's function is never
// mutated, so reusing one predicate across two routes never aliases their
// targets or marks. Fails when targets is empty.
func DefineRoute(targets []string, fn EdgePredicate, opts *DefineRouteOptions) (*EdgeFn, error) {
	if len(targets) == 0 {
		return nil, InvalidError("defineRoute", "targets must declare at least one possible return value")
	}
	wrapped := &EdgeFn{fn: fn, Targets: append([]string(nil), targets...)}
	if opts == nil || opts.ReadsData == nil || *opts.ReadsData {
		MarkSymbol(wrapped, ReadsData, true)
	}
	return wrapped, nil
}

// RouteNote is the symbol under which an EdgeFn records a note about its
// most recent pick — today: Gate / Match's "no branch matched" diagnostic.
// The runner's routing audit reads-and-clears it via TakeRouteNote right
// after invoking the edge and persists it on the routing row's note.
var RouteNote = SymbolFor("rpiv.workflow.routeNote")

// TakeRouteNote reads-and-clears the note an EdgeFn attached to its most
// recent invocation; false when the edge recorded nothing.
func TakeRouteNote(fn *EdgeFn) (string, bool) {
	if fn == nil {
		return "", false
	}
	note, ok := ReadSymbol[string](fn, RouteNote)
	if ok {
		MarkSymbol(fn, RouteNote, nil)
	}
	return note, ok
}

// SetRouteNote attaches a note to an EdgeFn's CURRENT pick — the
// authoring-surface dual of TakeRouteNote for bespoke DefineRoute gates. The
// routing audit reads-and-clears it in the same tick and persists it on the
// routing row, where SummarizeRun surfaces it as a stopped run's reason.
func SetRouteNote(fn *EdgeFn, note string) {
	MarkSymbol(fn, RouteNote, note)
}

// integerLikeKey matches the keys JavaScript engines hoist and reorder ahead
// of string keys (canonical array indices). Gate evaluates branches in
// declaration order, so an integer-like stage name would silently change
// match priority — rejected at construction.
var integerLikeKey = regexp.MustCompile(`^\d+$`)

// validateBranchKeys are the shared branch-key guards for Gate / Match.
func validateBranchKeys(factory string, branchTargets []string, renameHint string) error {
	if len(branchTargets) == 0 {
		return InvalidError(factory, "branches must declare at least one possible return value")
	}
	for _, key := range branchTargets {
		if integerLikeKey.MatchString(key) {
			return InvalidError(factory, "branch key \""+key+"\" is integer-like — JS reorders such keys ahead of declaration order, silently changing match priority. "+renameHint)
		}
	}
	return nil
}

// GateBranch is one Gate branch: the target stage and its predicate.
// Branches are an ordered slice because evaluation order IS the contract.
type GateBranch struct {
	Target string
	When   NumericPredicate
}

func uniqueTargets(keys []string, extra string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, k := range append(append([]string(nil), keys...), extra) {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// Gate is conditional routing keyed on a numeric field in output.data. Each
// branch's predicate is evaluated against Number(output.data[field]) in
// declaration order; the first match wins. When none matches (including a
// missing or non-numeric field, which coerces to NaN) the EXPLICIT otherwise
// branch is taken and the route note says the fallback fired. Branch keys
// must not be integer-like. Built on DefineRoute so Targets (otherwise
// included) and the ReadsData marker attach structurally.
func Gate(field string, branches []GateBranch, otherwise string) (*EdgeFn, error) {
	branchTargets := make([]string, len(branches))
	for i, b := range branches {
		branchTargets[i] = b.Target
	}
	if err := validateBranchKeys("gate", branchTargets, "Rename the stage or route to it via `otherwise`."); err != nil {
		return nil, err
	}
	if err := RequireNonEmptyString(otherwise, "gate", "an explicit `otherwise` branch is required — the no-match fallback must be deliberate", false); err != nil {
		return nil, err
	}
	targets := uniqueTargets(branchTargets, otherwise)
	var route *EdgeFn
	route, err := DefineRoute(targets, func(ctx EdgeContext) (string, error) {
		var raw any
		if ctx.Output != nil {
			raw, _ = fieldOf(ctx.Output.Data, field)
		}
		value := jsNumber(raw)
		for _, b := range branches {
			if b.When(value) {
				return b.Target, nil
			}
		}
		MarkSymbol(route, RouteNote, "gate(\""+field+"\"): no branch matched value "+formatJSNumber(value)+" — fell back to \""+otherwise+"\"")
		return otherwise, nil
	}, nil)
	return route, err
}

// MatchValue is a discrete value a Match branch compares its field against
// (strict equality): a string, a number or a bool.
type MatchValue = any

// MatchBranch is one Match branch: the target stage and the value routing to it.
type MatchBranch struct {
	Target string
	Value  MatchValue
}

// MatchOptions are the options for Match. Fallback is the stage to route to
// when no branch value matches (omitted ⇒ the unmatched value TERMINATES the
// chain). From reads the matched field from a NAMED CHANNEL's latest output
// instead of the stage's projected output — how a Match routes on a panel's
// published verdict; a channel-sourced route does not mark the source stage
// ReadsData.
type MatchOptions struct {
	Fallback string
	From     string
}

// Match is conditional routing keyed on an ENUM field — the string/boolean
// companion to the numeric Gate. The field is compared by strict equality in
// declaration order; the first match wins. No-match handling is EXPLICIT:
// with a Fallback the unmatched value routes there; without one it
// terminates (Stop). Both record a route note. Each value must map to exactly
// one stage, and branch keys must not be integer-like.
func Match(field string, branches []MatchBranch, opts *MatchOptions) (*EdgeFn, error) {
	branchTargets := make([]string, len(branches))
	for i, b := range branches {
		branchTargets[i] = b.Target
	}
	if err := validateBranchKeys("match", branchTargets, "Rename the stage."); err != nil {
		return nil, err
	}
	claimedBy := map[string]string{}
	for _, b := range branches {
		// Type-tag the value so 0/"0"/false stay distinct when deduping.
		valueKey := jsTypeof(b.Value) + ":" + jsString(b.Value)
		if prior, ok := claimedBy[valueKey]; ok {
			return nil, InvalidError("match", "value "+jsonStringify(b.Value)+" is claimed by both \""+prior+"\" and \""+b.Target+"\" — each enum value must map to exactly one stage")
		}
		claimedBy[valueKey] = b.Target
	}
	fallback, from := "", ""
	if opts != nil {
		fallback, from = opts.Fallback, opts.From
	}
	noMatch := fallback
	if noMatch == "" {
		noMatch = Stop
	}
	targets := uniqueTargets(branchTargets, noMatch)
	var routeOpts *DefineRouteOptions
	if from != "" {
		// A channel-sourced match validates the CHANNEL's data, not the source
		// stage's projected output — so don't demand an outputSchema there.
		routeOpts = &DefineRouteOptions{ReadsData: Bool(false)}
	}
	var route *EdgeFn
	route, err := DefineRoute(targets, func(ctx EdgeContext) (string, error) {
		var source any
		if from != "" {
			if ctx.State != nil {
				if history := ctx.State.Named()[from]; len(history) > 0 {
					source = history[len(history)-1].Data
				}
			}
		} else if ctx.Output != nil {
			source = ctx.Output.Data
		}
		raw, present := fieldOf(source, field)
		for _, b := range branches {
			if present && jsStrictEqual(raw, b.Value) {
				return b.Target, nil
			}
		}
		tail := "terminated (no fallback)"
		if fallback != "" {
			tail = "fell back to \"" + fallback + "\""
		}
		MarkSymbol(route, RouteNote, "match(\""+field+"\"): value "+jsonStringify(raw)+" matched no branch — "+tail)
		return noMatch, nil
	}, routeOpts)
	return route, err
}
