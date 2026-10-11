package mcp

import "time"

// SetNow replaces the clock of the check, so that a test does not wait one
// minute for the next fetch of the key set.
func (c *AccessCheck) SetNow(now func() time.Time) { c.now = now }
