// Copyright 2026, Jamf Software LLC

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Jamf-Concepts/jamformer/update"
)

// updateNoticeGrace is how long reportUpdateNotice waits for a check that has
// not finished. The result is cached for a day, so all but the first run each
// day resolves instantly; on that first run this bounds the delay the user
// sees. An unfinished check is dropped rather than waited out — a version
// notice is never worth holding up the export for.
const updateNoticeGrace = 500 * time.Millisecond

// startUpdateCheck kicks off the release check in the background and returns a
// channel carrying its single result.
func startUpdateCheck() <-chan update.Result {
	ch := make(chan update.Result, 1)
	go func() {
		defer close(ch)
		// A failed check is not reported: the network being unavailable is not
		// something the user asked about, and an export does not need GitHub.
		res, err := update.Check(context.Background(), version)
		if err != nil {
			return
		}
		ch <- res
	}()
	return ch
}

// reportUpdateNotice prints the upgrade advisory, if there is one, naming the
// command that fits how this binary was installed.
//
// It writes to stderr so that a run whose stdout is piped or redirected — a CI
// job capturing output, say — is not handed a version notice in the middle of
// its data.
func reportUpdateNotice(ch <-chan update.Result) {
	if ch == nil {
		return
	}
	var res update.Result
	select {
	case r, ok := <-ch:
		if !ok {
			return
		}
		res = r
	case <-time.After(updateNoticeGrace):
		return
	}

	notice := res.Notice()
	if notice == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "\n%s%s%s\n\n", uBold, notice, uReset)
}

// needsScopePrompt reports whether the interactive flow should ask which scope
// the API integration carries.
//
// It asks only when nothing has said: neither identifier is set and neither
// organization variable is exported. Organization scope is legitimately
// identifier-free, so an explicitly named organization ID has to count as an
// answer — otherwise the prompt would appear on every organization-scoped run
// and there would be no way to say "I meant that".
func needsScopePrompt(environmentID, tenantID string) bool {
	if environmentID != "" || tenantID != "" {
		return false
	}
	return os.Getenv("JAMFPLATFORM_ORGANIZATION_ID") == "" && os.Getenv("JAMF_ORGANIZATION_ID") == ""
}
