package file

import (
	pb "api/api/file/common/v1"

	"github.com/samber/lo"
)

type (
	ViewerAction string
	ViewerType   string
)

type (
	Viewer struct {
		ID                      string                             `json:"id"`
		Type                    ViewerType                         `json:"type"`
		DisplayName             string                             `json:"display_name"`
		Exts                    []string                           `json:"exts"`
		Url                     string                             `json:"url,omitempty"`
		Icon                    string                             `json:"icon,omitempty"`
		WopiActions             map[string]map[ViewerAction]string `json:"wopi_actions,omitempty"`
		Props                   map[string]string                  `json:"props,omitempty"`
		MaxSize                 int64                              `json:"max_size,omitempty"`
		Disabled                bool                               `json:"disabled,omitempty"`
		Templates               []NewFileTemplate                  `json:"templates,omitempty"`
		Platform                string                             `json:"platform,omitempty"`
		RequiredGroupPermission []int                              `json:"required_group_permission,omitempty"`
	}
	ViewerGroup struct {
		Viewers []Viewer `json:"viewers"`
	}

	NewFileTemplate struct {
		Ext         string `json:"ext"`
		DisplayName string `json:"display_name"`
	}
)

func ToProtoViewerGroup(viewerGroup *ViewerGroup) *pb.ViewerGroup {
	if viewerGroup == nil {
		return nil
	}
	return &pb.ViewerGroup{
		Viewers: lo.Map(viewerGroup.Viewers, func(item Viewer, index int) *pb.Viewer {
			return ToProtoViewer(&item)
		}),
	}
}

func ToProtoViewer(viewer *Viewer) *pb.Viewer {
	if viewer == nil {
		return nil
	}
	return &pb.Viewer{
		Id:          viewer.ID,
		DisplayName: viewer.DisplayName,
		Type:        string(viewer.Type),
		Exts:        viewer.Exts,
		Url:         viewer.Url,
		Icon:        viewer.Icon,
		WopiActions: lo.MapEntries(viewer.WopiActions, func(key string, value map[ViewerAction]string) (string, *pb.WopiActionMap) {
			return key, &pb.WopiActionMap{
				Actions: lo.MapEntries(value, func(key ViewerAction, value string) (string, string) {
					return string(key), value
				}),
			}
		}),
		Props: viewer.Props,
	}
}
