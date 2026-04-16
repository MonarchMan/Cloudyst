package admin

import (
	commonpb "api/api/common/v1"
	pb "api/api/file/admin/v1"
	filepb "api/api/file/common/v1"
	"context"
	"file/ent"
	"file/ent/node"
	"file/internal/data"
	"file/internal/pkg/utils"
	"strings"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	nodeStatusCondition = "node_status"
)

func (s *AdminService) ListNodes(ctx context.Context, req *filepb.ListRequest) (*pb.ListNodesResponse, error) {
	nodeClient := s.nc

	newCtx := context.WithValue(ctx, data.LoadNodeStoragePolicy{}, true)
	res, err := nodeClient.ListNodes(newCtx, &data.ListNodeParameters{
		PaginationArgs: &data.PaginationArgs{
			Page:     int(req.Page) - 1,
			PageSize: int(req.PageSize),
			OrderBy:  req.OrderBy,
			Order:    data.OrderDirection(req.OrderDirection),
		},
		Status: node.Status(req.Conditions[nodeStatusCondition]),
	})

	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list nodes: %w", err)
	}

	return &pb.ListNodesResponse{
		Nodes: lo.Map(res.Nodes, func(item *ent.Node, index int) *filepb.Node {
			return utils.EntNodeToProto(item)
		}),
		Pagination: res.PaginationResults,
	}, nil
}
func (s *AdminService) GetNode(ctx context.Context, req *pb.SimpleNodeRequest) (*pb.GetNodeResponse, error) {
	nodeClient := s.nc
	newCtx := context.WithValue(ctx, data.LoadNodeStoragePolicy{}, true)
	node, err := nodeClient.GetNodeById(newCtx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get node: %w", err)
	}

	return &pb.GetNodeResponse{
		Node: utils.EntNodeToProto(node),
	}, nil
}
func (s *AdminService) TestSlave(ctx context.Context, req *pb.OpNodeRequest) (*emptypb.Empty, error) {
	//settings := s.dep.SettingProvider()
	//slave, err := url.Parse(req.Node.Server.Value)
	//if err != nil {
	//	return nil, response.WithReason(response.ErrParam, "Failed to parse node url", err)
	//}
	//
	//primaryURL := settings.SiteURL(setting.UseFirstSiteUrl(ctx)).String()
	//body := map[string]string{
	//	"callback": primaryURL,
	//}
	//bodyByte, _ := json.Marshal(body)
	return &emptypb.Empty{}, nil
}
func (s *AdminService) TestDownloader(ctx context.Context, req *pb.OpNodeRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
func (s *AdminService) CreateNode(ctx context.Context, req *pb.OpNodeRequest) (*pb.GetNodeResponse, error) {
	nodeClient := s.nc

	if req.Node.Id > 0 {
		return nil, commonpb.ErrorParamInvalid("ID must be 0")
	}

	node, _ := nodeClient.Upsert(ctx, utils.ProtoNodeToEnt(req.Node))

	// 重新加载节点池
	//np, err := s.dep.NodePool(ctx)
	//if err != nil {
	//	return nil, response.WithReason(response.ErrInternalSetting, "Failed to get node pool", err)
	//}
	s.np.Upsert(ctx, node)
	newReq := &pb.SimpleNodeRequest{Id: int32(node.ID)}
	return s.GetNode(ctx, newReq)
}
func (s *AdminService) UpdateNode(ctx context.Context, req *pb.OpNodeRequest) (*pb.GetNodeResponse, error) {
	nodeClient := s.nc
	if req.Node.Id == 0 {
		return nil, commonpb.ErrorParamInvalid("ID is required")
	}

	node, err := nodeClient.Upsert(ctx, utils.ProtoNodeToEnt(req.Node))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update node: %w", err)
	}

	s.np.Upsert(ctx, node)

	// 清空策略缓存，因为这个节点可能被某个存储策略缓存了
	s.kv.Delete(data.StoragePolicyCacheKey)
	newReq := &pb.SimpleNodeRequest{Id: int32(node.ID)}
	return s.GetNode(ctx, newReq)
}
func (s *AdminService) DeleteNode(ctx context.Context, req *pb.SimpleNodeRequest) (*emptypb.Empty, error) {
	nodeClient := s.nc
	newCtx := context.WithValue(ctx, data.LoadNodeStoragePolicy{}, true)
	existing, err := nodeClient.GetNodeById(newCtx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get node: %w", err)
	}

	if existing.Type == node.TypeMaster {
		return nil, filepb.ErrorInvalidActionOnSystemNode("Cannot delete master node")
	}

	if len(existing.Edges.StoragePolicy) > 0 {
		msg := strings.Join(lo.Map(existing.Edges.StoragePolicy, func(i *ent.StoragePolicy, _ int) string {
			return i.Name
		}), ", ")
		return nil, filepb.ErrorNodeUsedByStorePolicy(msg)
	}

	// 节点池内插入假的被禁止的节点以去除它
	disableNode := &ent.Node{
		ID:     int(req.Id),
		Status: node.StatusSuspended,
		Type:   node.TypeSlave,
	}
	s.np.Upsert(ctx, disableNode)
	return &emptypb.Empty{}, nodeClient.Delete(ctx, int(req.Id))
}
