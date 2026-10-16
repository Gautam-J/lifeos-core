package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Gautam-J/lifeos-core/internal/service"
)

// entitiesHelp is the sentence that each write tool puts at the end of its
// description (#41).
const entitiesHelp = " Give entities for each person, place, project and topic that the message names: the record is linked to each one, and an entity that does not exist yet is made when you give its kind. Ask no question about an entity. The result lists the links in entities: tell the user about each new_entity, so that a wrong one is easy to correct."

// entitiesInput is the input that the five write tools share.
type entitiesInput struct {
	Entities []service.EntityRef `json:"entities,omitempty" jsonschema:"The people, places, projects and topics that the record is about. Each one is {kind, name} (the entity is made when there is none), {name} (an entity that exists, by name or alias) or {id}."`
}

// entitiesOutput is the part of the result of a write tool that reports the
// links. The record is stored before the links are made, so a refused
// entity does not lose the record.
type entitiesOutput struct {
	Entities      []service.EntityLink `json:"entities,omitempty" jsonschema:"The entities that this call linked to the record. new_entity says that the call made the entity."`
	EntitiesError string               `json:"entities_error,omitempty" jsonschema:"Set when the entities could not be linked. The record is stored."`
}

// linkEntities links a stored record to the entities of the input.
func linkEntities(ctx context.Context, svc *service.Service, recordType string, id int64, refs []service.EntityRef) entitiesOutput {
	if len(refs) == 0 {
		return entitiesOutput{}
	}
	result, err := svc.Tag(ctx, service.TagParams{Type: recordType, ID: id, Add: refs, Actor: service.SourceAgent})
	if err != nil {
		return entitiesOutput{EntitiesError: "The record is stored, but its entities are not linked: " + toToolError(err).Error() + " Call tag for this record to link them."}
	}
	return entitiesOutput{Entities: result.Added}
}

type tagInput struct {
	Type   string              `json:"type" jsonschema:"Record type. One of: item, journal, todo, habit_log, source"`
	ID     int64               `json:"id" jsonschema:"Record ID"`
	Add    []service.EntityRef `json:"add,omitempty" jsonschema:"Entities to link to the record. Each one is {kind, name} (the entity is made when there is none), {name} or {id}."`
	Remove []service.EntityRef `json:"remove,omitempty" jsonschema:"Entities whose link to the record you take away. Each one is {name} or {id}."`
}

type entityInput struct {
	Kind     string   `json:"kind" jsonschema:"One of person, place, project, topic"`
	Name     string   `json:"name" jsonschema:"The name, as the user says it. The full name of a person."`
	Aliases  []string `json:"aliases,omitempty" jsonschema:"Other names of the entity, such as a short name or a nickname"`
	WikiPath string   `json:"wiki_path,omitempty" jsonschema:"The wiki page of the entity, such as people/ravi-kumar.md. Only a page that exists or that the user said yes to."`
}

type mergeEntitiesInput struct {
	Keep int64 `json:"keep" jsonschema:"The ID of the entity that stays"`
	Drop int64 `json:"drop" jsonschema:"The ID of the entity that goes into keep: it is the same person, place, project or topic under a second name"`
}

func registerEntities(server *sdk.Server, svc *service.Service) {
	sdk.AddTool(server, &sdk.Tool{
		Name:        "tag",
		Description: "Link a record that exists to people, places, projects and topics, or take a link away. Use it for a record that was written without entities, and to correct a wrong link that a result listed. Give add, remove or both. The same link a second time changes nothing. Returns the links added and removed. To read everything that links to one entity, use get with the type entity.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in tagInput) (*sdk.CallToolResult, service.TagResult, error) {
		result, err := svc.Tag(ctx, service.TagParams{Type: in.Type, ID: in.ID, Add: in.Add, Remove: in.Remove, Actor: service.SourceAgent})
		if err != nil {
			return nil, service.TagResult{}, toToolError(err)
		}
		return nil, result, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "entity",
		Description: "Add a person, place, project or topic to the list of entities, with its aliases. Use it for an entity that you want to make before a record names it. A record tool makes an entity by itself when you give it the kind, so this tool is rarely needed. When the name or an alias exists already, the error names the entity: use update with the type entity and add_aliases to add an alias. Returns the stored entity with its ID.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in entityInput) (*sdk.CallToolResult, service.Entity, error) {
		entity, err := svc.AddEntity(ctx, service.AddEntityParams{
			Kind:       in.Kind,
			Name:       in.Name,
			Aliases:    in.Aliases,
			WikiPath:   in.WikiPath,
			Provenance: service.Provenance{Source: service.SourceAgent},
		})
		if err != nil {
			return nil, service.Entity{}, toToolError(err)
		}
		return nil, entity, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "merge_entities",
		Description: "Merge two entities of one kind that are the same person, place, project or topic, such as \"R\" and \"Ravi Kumar\". Use it when you see that you made a second entity for something that exists. Every link and every project of a todo moves from drop to keep; the name and the aliases of drop become aliases of keep; drop is deleted. Be sure: a wrong merge can be undone only by hand. Returns what moved.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in mergeEntitiesInput) (*sdk.CallToolResult, service.MergeResult, error) {
		result, err := svc.MergeEntities(ctx, service.MergeEntitiesParams{Keep: in.Keep, Drop: in.Drop, Actor: service.SourceAgent})
		if err != nil {
			return nil, service.MergeResult{}, toToolError(err)
		}
		return nil, result, nil
	})
}
