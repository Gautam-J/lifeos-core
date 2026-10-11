package main

import (
	"fmt"
	"os"
	"regexp"

	lifeosmcp "github.com/Gautam-J/lifeos-core/internal/mcp"
)

var (
	accessTeamPattern = regexp.MustCompile(`^[a-z0-9-]+$`)
	accessAudPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// accessCheck returns the check of the Cloudflare Access token for the MCP
// server. The team name comes from CF_ACCESS_TEAM and the audience tag of
// the Access application from LIFEOS_MCP_AUD (deploy/.env). The two values
// are not secrets, but they name the Cloudflare account, so they are not in
// the config file, which is in git. The server does not start without
// them: the MCP server has no mode that needs no token.
func accessCheck() (*lifeosmcp.AccessCheck, error) {
	team, aud := os.Getenv("CF_ACCESS_TEAM"), os.Getenv("LIFEOS_MCP_AUD")
	if !accessTeamPattern.MatchString(team) {
		return nil, fmt.Errorf("CF_ACCESS_TEAM: %q is not the team name of Cloudflare Access, such as my-team", team)
	}
	if !accessAudPattern.MatchString(aud) {
		return nil, fmt.Errorf("LIFEOS_MCP_AUD: the value is not an audience tag of an Access application, which has 64 characters from 0-9 and a-f")
	}
	return lifeosmcp.NewAccessCheck("https://"+team+".cloudflareaccess.com", aud), nil
}
