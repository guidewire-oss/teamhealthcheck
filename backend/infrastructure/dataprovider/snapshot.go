package dataprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

// snapshotPath is the provider's snapshot endpoint. It is not configurable:
// the contract, not the deployment, decides where the snapshot lives. See
// docs/organization-snapshot-contract.md.
const snapshotPath = "/org-snapshot"

// maxSnapshotBytes caps how much of a snapshot response is read, so a
// misbehaving or hostile upstream cannot exhaust memory. It is a variable
// rather than a constant so tests can exercise the limit without generating a
// payload of this size.
var maxSnapshotBytes int64 = 64 << 20 // 64 MiB

// ErrSnapshotTooLarge is returned when a provider response exceeds
// maxSnapshotBytes. The response is rejected outright rather than decoded from
// a truncated body, so an oversized payload can never be mistaken for a
// complete snapshot.
var ErrSnapshotTooLarge = errors.New("dataprovider: provider response exceeds the maximum snapshot size")

// FetchSnapshot issues a single GET to the provider's snapshot endpoint through
// Do and decodes the provider-neutral organization snapshot contract.
//
// Errors never include the response body or the API token. An upstream failure
// can echo request headers or embed record data, and this error text reaches an
// admin's browser -- so only the status code is reported.
func (c *Client) FetchSnapshot(ctx context.Context) (*orgsnapshot.Snapshot, error) {
	resp, err := c.Do(ctx, http.MethodGet, snapshotPath, nil)
	if err != nil {
		// Do already sanitizes the underlying transport error (see
		// ErrRequestFailed), so err carries no request URL or credential here.
		return nil, fmt.Errorf("dataprovider: provider request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Drain a bounded amount so the connection can be reused, but discard it.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("dataprovider: provider returned HTTP %d", resp.StatusCode)
	}

	// Read one byte past the limit so an oversized body is rejected rather than
	// decoded from a truncated prefix.
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxSnapshotBytes+1))
	if err != nil {
		return nil, errors.New("dataprovider: failed to read the provider response")
	}
	if int64(len(payload)) > maxSnapshotBytes {
		return nil, ErrSnapshotTooLarge
	}

	var snapshot orgsnapshot.Snapshot
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		// The decode error can quote the offending JSON fragment, which is
		// provider record data. Report the shape of the failure only.
		return nil, errors.New("dataprovider: provider response was not a valid organization snapshot")
	}

	return &snapshot, nil
}
