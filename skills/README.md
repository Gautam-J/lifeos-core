# Skill drafts

These seven files are drafts. No agent loads them.

They were written in the `lifeos-wiki` repository before the MCP tools existed. They are here unchanged, because `lifeos-wiki` holds only the data of the user.

The rules for an agent are in the tool descriptions, in the `next` field of the results of `journal` and `capture`, and in `internal/mcp/instructions.go` (rule 14 of [AGENTS.md](../AGENTS.md)). A draft becomes a skill only when daily use shows a procedure that the agent does wrong with those rules alone.

Correct a draft against the tools before you use it. Known differences:

- The marks `[planned]` and `[missing]` are out of date. Compare each tool with the tool list of the MCP server.
- The server writes `index.md`. An agent does not add a page there.
- A citation prefix is one of `WikiCitationPrefixes`. The drafts use prefixes that do not exist, such as `(visit:311)`.
- A record that an agent infers gets `dictated=false`.
