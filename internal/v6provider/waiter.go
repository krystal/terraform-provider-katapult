package v6provider

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// All of this is lifted from plugin SDKv2
// There is no equivalent in the Framework provider.

type (
	NotFoundError struct {
		LastError    error
		LastRequest  interface{}
		LastResponse interface{}
		Message      string
		Retries      int
	}

	// UnexpectedStateError is returned when Refresh returns
	// a state that's neither in Target nor Pending.
	UnexpectedStateError struct {
		LastError     error
		State         string
		ExpectedState []string
	}

	// TimeoutError is returned when WaitForState times out.
	TimeoutError struct {
		LastError     error
		LastState     string
		Timeout       time.Duration
		ExpectedState []string
	}

	StateRefreshFunc func() (result interface{}, state string, err error)

	Waiter struct {
		// Wait this time before starting checks
		Delay time.Duration
		// States that are "allowed" and will continue trying
		Pending []string
		// Refreshes the current state
		Refresh StateRefreshFunc
		// Target state
		Target []string
		// The amount of time to wait before timeout
		Timeout time.Duration
		// Smallest time to wait before refreshes
		MinTimeout time.Duration
		// Override MinTimeout/backoff and only poll this often
		PollInterval time.Duration
		// Number of times to allow not found (nil result from Refresh)
		NotFoundChecks int

		// This is to work around inconsistent APIs
		// Number of times the Target state has to occur continuously
		ContinuousTargetOccurence int
	}
)

var refreshGracePeriod = 30 * time.Second

func (e *NotFoundError) Error() string {
	if e.Message != "" {
		return e.Message
	}

	if e.Retries > 0 {
		return fmt.Sprintf("couldn't find resource (%d retries)", e.Retries)
	}

	return "couldn't find resource"
}

func (e *NotFoundError) Unwrap() error {
	return e.LastError
}

func (e *UnexpectedStateError) Error() string {
	return fmt.Sprintf(
		"unexpected state '%s', wanted target '%s'. last error: %s",
		e.State,
		strings.Join(e.ExpectedState, ", "),
		e.LastError,
	)
}

func (e *UnexpectedStateError) Unwrap() error {
	return e.LastError
}

func (e *TimeoutError) Error() string {
	expectedState := "resource to be gone"
	if len(e.ExpectedState) > 0 {
		expectedState = fmt.Sprintf(
			"state to become '%s'",
			strings.Join(e.ExpectedState, ", "),
		)
	}

	extraInfo := make([]string, 0)
	if e.LastState != "" {
		extraInfo = append(extraInfo,
			fmt.Sprintf("last state: '%s'", e.LastState))
	}
	if e.Timeout > 0 {
		extraInfo = append(extraInfo,
			fmt.Sprintf("timeout: %s", e.Timeout.String()))
	}

	suffix := ""
	if len(extraInfo) > 0 {
		suffix = fmt.Sprintf(" (%s)", strings.Join(extraInfo, ", "))
	}

	if e.LastError != nil {
		return fmt.Sprintf("timeout while waiting for %s%s: %s",
			expectedState, suffix, e.LastError)
	}

	return fmt.Sprintf("timeout while waiting for %s%s",
		expectedState, suffix)
}

func (e *TimeoutError) Unwrap() error {
	return e.LastError
}

//nolint:funlen,gocyclo // this is lifted from the SDK
func (w *Waiter) WaitForStateContext(ctx context.Context) (interface{}, error) {
	log.Printf("[DEBUG] Waiting for state to become: %s", w.Target)

	notfoundTick := 0
	targetOccurence := 0

	// Set a default for times to check for not found
	if w.NotFoundChecks == 0 {
		w.NotFoundChecks = 20
	}

	if w.ContinuousTargetOccurence == 0 {
		w.ContinuousTargetOccurence = 1
	}

	type Result struct {
		Result interface{}
		State  string
		Error  error
		Done   bool
	}

	// Read every result from the refresh loop,
	// waiting for a positive result.Done.
	resCh := make(chan Result, 1)
	// cancellation channel for the refresh loop
	cancelCh := make(chan struct{})

	result := Result{}

	go func() {
		defer close(resCh)

		select {
		case <-time.After(w.Delay):
		case <-cancelCh:
			return
		}

		// start with 0 delay for the first loop
		var wait time.Duration

		for {
			// store the last result
			resCh <- result

			// wait and watch for cancellation
			select {
			case <-cancelCh:
				return
			case <-time.After(wait):
				// first round had no wait
				if wait == 0 {
					wait = 100 * time.Millisecond
				}
			}

			res, currentState, err := w.Refresh()
			result = Result{
				Result: res,
				State:  currentState,
				Error:  err,
			}

			if err != nil {
				resCh <- result
				return
			}

			// If we're waiting for the absence of a thing, then return
			if res == nil && len(w.Target) == 0 {
				targetOccurence++
				if w.ContinuousTargetOccurence == targetOccurence {
					result.Done = true
					resCh <- result
					return
				}
				continue
			}

			if res == nil {
				// If we didn't find the resource, check if we have been

				// not finding it for awhile, and if so, report an error.
				notfoundTick++
				if notfoundTick > w.NotFoundChecks {
					result.Error = &NotFoundError{
						LastError: err,
						Retries:   notfoundTick,
					}
					resCh <- result
					return
				}
			} else {
				// Reset the counter for when a resource isn't found
				notfoundTick = 0
				found := false

				for _, allowed := range w.Target {
					if currentState == allowed {
						found = true
						targetOccurence++
						if w.ContinuousTargetOccurence == targetOccurence {
							result.Done = true
							resCh <- result
							return
						}
						continue
					}
				}

				for _, allowed := range w.Pending {
					if currentState == allowed {
						found = true
						targetOccurence = 0
						break
					}
				}

				if !found && len(w.Pending) > 0 {
					result.Error = &UnexpectedStateError{
						LastError:     err,
						State:         result.State,
						ExpectedState: w.Target,
					}
					resCh <- result
					return
				}
			}

			// Wait between refreshes using exponential backoff, except when
			// waiting for the target state to reoccur.
			if targetOccurence == 0 {
				wait *= 2
			}

			// If a poll interval has been specified, choose that interval.
			// Otherwise bound the default value.
			if w.PollInterval > 0 && w.PollInterval < 180*time.Second {
				wait = w.PollInterval
			} else {
				if wait < w.MinTimeout {
					wait = w.MinTimeout
				} else if wait > 10*time.Second {
					wait = 10 * time.Second
				}
			}

			log.Printf("[TRACE] Waiting %s before next try", wait)
		}
	}()

	// store the last value result from the refresh loop
	lastResult := Result{}

	timeout := time.After(w.Timeout)
	for {
		select {
		case r, ok := <-resCh:
			// channel closed, so return the last result
			if !ok {
				return lastResult.Result, lastResult.Error
			}

			// we reached the intended state
			if r.Done {
				return r.Result, r.Error
			}

			// still waiting, store the last result
			lastResult = r
		case <-ctx.Done():
			close(cancelCh)
			return nil, ctx.Err()
		case <-timeout:
			log.Printf("[WARN] WaitForState timeout after %s", w.Timeout)
			log.Printf("[WARN] WaitForState starting %s refresh grace period",
				refreshGracePeriod)

			// cancel the goroutine and start our grace period timer
			close(cancelCh)
			timeout := time.After(refreshGracePeriod)

			// we need a for loop and a label to break on, because we may have
			// an extra response value to read, but still want to wait for the
			// channel to close.
		forSelect:
			for {
				select {
				case r, ok := <-resCh:
					if r.Done {
						// the last refresh loop reached the desired state
						return r.Result, r.Error
					}

					if !ok {
						// the goroutine returned
						break forSelect
					}

					// target state not reached, save the result for the
					// TimeoutError and wait for the channel to close
					lastResult = r
				case <-ctx.Done():
					log.Println(
						"[ERROR] Context cancelation detected," +
							"abandoning grace period")
					break forSelect
				case <-timeout:
					log.Println(
						"[ERROR] WaitForState exceeded refresh grace period")
					break forSelect
				}
			}

			return nil, &TimeoutError{
				LastError:     lastResult.Error,
				LastState:     lastResult.State,
				Timeout:       w.Timeout,
				ExpectedState: w.Target,
			}
		}
	}
}
