package filedata

import (
	pb "api/api/common/v1"
	filepb "api/api/file/common/v1"

	"github.com/samber/lo"
)

func ExplorerViewFromProto(f *filepb.ExplorerView) *ExplorerView {
	if f == nil {
		return nil
	}

	return &ExplorerView{
		PageSize:       int(f.PageSize),
		Order:          f.Order,
		OrderDirection: f.OrderDirection,
		View:           ViewTypeFromProto(f.View),
		Thumbnail:      f.Thumbnail,
		GalleryWidth:   int(f.GalleryWidth),
		Columns: lo.Map(f.Columns, func(item *filepb.ListViewColumn, index int) ListViewColumn {
			return *ListVIewColumnFromProto(item)
		}),
	}
}

func ListVIewColumnFromProto(column *filepb.ListViewColumn) *ListViewColumn {
	if column == nil {
		return nil
	}

	c := &ListViewColumn{
		Type:  int(column.Type),
		Props: ColumnTypePropsFromProto(column.Props),
	}
	if column.Width != nil {
		width := int(*column.Width)
		c.Width = &width
	}
	return c
}

func ColumnTypePropsFromProto(props *filepb.ColumnTypeProps) *ColumTypeProps {
	if props == nil {
		return nil
	}
	return &ColumTypeProps{
		MetadataKey:   props.MetadataKey,
		CustomPropsID: props.CustomPropsId,
	}
}

func ViewTypeFromProto(viewType filepb.ViewType) string {
	switch viewType {
	case filepb.ViewType_VIEW_TYPE_LIST:
		return "list"
	case filepb.ViewType_VIEW_TYPE_GRID:
		return "grid"
	case filepb.ViewType_VIEW_TYPE_GALLERY:
		return "gallery"
	default:
		return ""
	}
}

func ExplorerViewToProto(view *ExplorerView) *filepb.ExplorerView {
	if view == nil {
		return nil
	}
	return &filepb.ExplorerView{
		PageSize:       int32(view.PageSize),
		Order:          view.Order,
		OrderDirection: view.OrderDirection,
		View:           ViewTypeToProto(view.View),
		Thumbnail:      view.Thumbnail,
		GalleryWidth:   int32(view.GalleryWidth),
		Columns: lo.Map(view.Columns, func(item ListViewColumn, _ int) *filepb.ListViewColumn {
			return ListVIewColumnToProto(&item)
		}),
	}
}

func ListVIewColumnToProto(column *ListViewColumn) *filepb.ListViewColumn {
	if column == nil {
		return nil
	}
	c := &filepb.ListViewColumn{
		Type:  int32(column.Type),
		Props: ColumnTypePropsToProto(column.Props),
	}
	if column.Width != nil {
		width := int32(*column.Width)
		c.Width = &width
	}
	return c
}

func ColumnTypePropsToProto(props *ColumTypeProps) *filepb.ColumnTypeProps {
	if props == nil {
		return nil
	}
	return &filepb.ColumnTypeProps{
		MetadataKey:   props.MetadataKey,
		CustomPropsId: props.CustomPropsID,
	}
}

func ViewTypeToProto(viewType string) filepb.ViewType {
	switch viewType {
	case "list":
		return filepb.ViewType_VIEW_TYPE_LIST
	case "grid":
		return filepb.ViewType_VIEW_TYPE_GRID
	case "gallery":
		return filepb.ViewType_VIEW_TYPE_GALLERY
	default:
		return filepb.ViewType_VIEW_TYPE_LIST
	}
}

func StoragePolicyInfoToProto(info *StoragePolicyInfo) *pb.StoragePolicyInfo {
	if info == nil {
		return nil
	}
	return &pb.StoragePolicyInfo{
		Id:        int32(info.ID),
		Name:      info.Name,
		Type:      info.Type,
		IsPrivate: info.IsPrivate,
	}
}

func StoragePolicyInfoFromProto(info *pb.StoragePolicyInfo) *StoragePolicyInfo {
	if info == nil {
		return nil
	}
	return &StoragePolicyInfo{
		ID:        int(info.Id),
		Name:      info.Name,
		Type:      info.Type,
		IsPrivate: info.IsPrivate,
	}
}
