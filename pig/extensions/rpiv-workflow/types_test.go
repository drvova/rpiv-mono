package workflow

import (
	"encoding/json"
	"testing"
)

func TestRunTerminationJSON(t *testing.T) {
	cases := []struct {
		term RunTermination
		want string
	}{
		{RunTermination{Status: TerminationRunning}, `{"status":"running"}`},
		{RunTermination{Status: TerminationCompleted}, `{"status":"completed"}`},
		{RunTermination{Status: TerminationFailed}, `{"status":"failed","error":""}`},
		{RunTermination{Status: TerminationFailed, Error: "boom"}, `{"status":"failed","error":"boom"}`},
		{RunTermination{Status: TerminationAborted, Error: ""}, `{"status":"aborted","error":""}`},
		{RunTermination{Status: TerminationCancelled, Error: "user"}, `{"status":"cancelled","error":"user"}`},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			raw, err := json.Marshal(c.term)
			if err != nil || string(raw) != c.want {
				t.Fatalf("got %s (%v)", raw, err)
			}
			var back RunTermination
			if err := json.Unmarshal(raw, &back); err != nil || back != c.term {
				t.Fatalf("round trip %+v (%v)", back, err)
			}
		})
	}
}
