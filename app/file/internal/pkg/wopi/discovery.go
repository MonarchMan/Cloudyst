package wopi

import (
	ftypes "api/external/data/file"
	"encoding/xml"
	"file/internal/data/types"
	"fmt"

	"github.com/gofrs/uuid"
	"github.com/samber/lo"
)

type ActonType string

var (
	ActionPreview         = ActonType("embedview")
	ActionPreviewFallback = ActonType("view")
	ActionEdit            = ActonType("edit")
)

func DiscoveryXmlToViewerGroup(xmlStr string) (*ftypes.ViewerGroup, error) {
	var discovery WopiDiscovery
	if err := xml.Unmarshal([]byte(xmlStr), &discovery); err != nil {
		return nil, fmt.Errorf("failed to parse WOPI discovery XML: %w", err)
	}

	group := &ftypes.ViewerGroup{
		Viewers: make([]ftypes.Viewer, 0, len(discovery.NetZone.App)),
	}

	for _, app := range discovery.NetZone.App {
		viewer := ftypes.Viewer{
			ID:          uuid.Must(uuid.NewV4()).String(),
			DisplayName: app.Name,
			Type:        types.ViewerTypeWopi,
			Icon:        app.FavIconUrl,
			WopiActions: make(map[string]map[ftypes.ViewerAction]string),
		}

		for _, action := range app.Action {
			if action.Ext == "" {
				continue
			}

			if _, ok := viewer.WopiActions[action.Ext]; !ok {
				viewer.WopiActions[action.Ext] = make(map[ftypes.ViewerAction]string)
			}

			if action.Name == string(ActionPreview) {
				viewer.WopiActions[action.Ext][types.ViewerActionView] = action.Urlsrc
			} else if action.Name == string(ActionPreviewFallback) {
				viewer.WopiActions[action.Ext][types.ViewerActionView] = action.Urlsrc
			} else if action.Name == string(ActionEdit) {
				viewer.WopiActions[action.Ext][types.ViewerActionEdit] = action.Urlsrc
			} else if len(viewer.WopiActions[action.Ext]) == 0 {
				delete(viewer.WopiActions, action.Ext)
			}
		}

		viewer.Exts = lo.MapToSlice(viewer.WopiActions, func(key string, value map[ftypes.ViewerAction]string) string {
			return key
		})

		if len(viewer.WopiActions) > 0 {
			group.Viewers = append(group.Viewers, viewer)
		}
	}

	return group, nil
}
