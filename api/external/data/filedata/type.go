package filedata

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

type (
	ExplorerView struct {
		PageSize       int              `json:"page_size" binding:"min=50"`
		Order          string           `json:"order,omitempty" binding:"max=255"`
		OrderDirection string           `json:"order_direction,omitempty" binding:"eq=asc|eq=desc"`
		View           string           `json:"view,omitempty" binding:"eq=list|eq=grid|eq=gallery"`
		Thumbnail      bool             `json:"thumbnail,omitempty"`
		GalleryWidth   int              `json:"gallery_width,omitempty" binding:"min=50,max=500"`
		Columns        []ListViewColumn `json:"columns,omitempty" binding:"max=1000"`
	}

	ListViewColumn struct {
		Type  int             `json:"type" binding:"min=0"`
		Width *int            `json:"width,omitempty"`
		Props *ColumTypeProps `json:"props,omitempty"`
	}

	ColumTypeProps struct {
		MetadataKey   string `json:"metadata_key,omitempty" binding:"max=255"`
		CustomPropsID string `json:"custom_props_id,omitempty" binding:"max=255"`
	}
)

type StoragePolicyInfo struct {
	ID        int    `json:"id" binding:"min=0"`
	Name      string `json:"name" binding:"max=255"`
	Type      string `json:"type" binding:"max=255"`
	IsPrivate bool   `json:"is_private,omitempty"`
}
