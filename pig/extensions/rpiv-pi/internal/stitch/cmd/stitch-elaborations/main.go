// Command stitch-elaborations splices per-phase elaboration docs back into a synthesized plan.
//
// Usage: stitch-elaborations <plan-path>
//
//	<plan-path> — a plan under .rpiv/artifacts/plans/ (relative paths resolve against the git root,
//	              then cwd).
//
// It prints the run's summary line on stdout and exits 0 on a normal run (a plan phase with no
// elaboration is left as-is and reported — partial runs are allowed). It exits 1 on a refused
// elaboration or a wiring/path error: missing argument, missing plan, or zero elaboration docs found.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/juicesharp/rpiv-mono/pig/extensions/rpiv-pi/internal/stitch"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the CLI over args (the arguments after the program name) and returns its exit code.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "" {
		fmt.Fprintln(stderr, "stitch-elaborations: missing <plan-path>")
		return 1
	}
	result, err := stitch.Stitch(args[0], stitch.Options{})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, result.Report())
	return 0
}
