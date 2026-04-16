package admin

import (
	commonpb "api/api/common/v1"
	pb "api/api/file/admin/v1"
	filepb "api/api/file/common/v1"
	"common/util"
	"context"
	"errors"
	"file/ent"
	"file/internal/biz/cluster/routes"
	"file/internal/biz/credmanager"
	"file/internal/biz/filemanager/driver/onedrive"
	"file/internal/biz/filemanager/manager"
	"file/internal/data"
	"file/internal/data/types"
	"file/internal/pkg/utils"
	"fmt"
	"strconv"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	policyTypeCondition = "policy_type"
	countEntityQuery    = "countEntity"
)

func (s *AdminService) ListPolicies(ctx context.Context, req *filepb.ListRequest) (*pb.ListStoragePoliciesResponse, error) {
	newCtx := context.WithValue(ctx, data.LoadStoragePolicyGroup{}, true)
	res, err := s.pc.ListPolicies(newCtx, &data.ListPolicyParameters{
		PaginationArgs: &data.PaginationArgs{
			Page:     int(req.Page) - 1,
			PageSize: int(req.PageSize),
			OrderBy:  req.OrderBy,
			Order:    data.OrderDirection(req.OrderDirection),
		},
		Type: types.PolicyType(req.Conditions[policyTypeCondition]),
	})
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list policies: %w", err)
	}

	return &pb.ListStoragePoliciesResponse{
		Pagination: res.PaginationResults,
		Policies: lo.Map(res.Policies, func(policy *ent.StoragePolicy, _ int) *filepb.StoragePolicy {
			return utils.EntPolicyToProto(policy)
		}),
	}, nil
}
func (s *AdminService) GetPolicy(ctx context.Context, req *pb.GetStoragePolicyRequest) (*pb.GetStoragePolicyResponse, error) {
	newCtx := context.WithValue(ctx, data.LoadStoragePolicyGroup{}, true)
	newCtx = context.WithValue(newCtx, data.SkipStoragePolicyCache{}, true)
	policy, err := s.pc.GetPolicyByID(newCtx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get policy: %w", err)
	}

	res := &pb.GetStoragePolicyResponse{StoragePolicy: utils.EntPolicyToProto(policy)}
	if req.CountEntity {
		count, size, err := s.fc.CountEntityByStoragePolicyID(ctx, int(req.Id))
		if err != nil {
			return nil, commonpb.ErrorDb("Failed to count entities: %w", err)
		}
		res.EntitiesCount = int32(count)
		res.EntitiesSize = int64(size)
	}

	return res, nil
}
func (s *AdminService) CreatePolicy(ctx context.Context, req *pb.CreateStoragePolicyRequest) (*pb.GetStoragePolicyResponse, error) {
	storagePolicyClient := s.pc

	if req.Policy.Type != types.PolicyTypeLocal {
		req.Policy.DirNameRule = &wrapperspb.StringValue{Value: util.DataPath("uploads/{uid}/{path}")}
	}

	req.Policy.Id = 0
	policy, err := storagePolicyClient.Upsert(ctx, utils.ProtoPolicyToEnt(req.Policy))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create policy: %w", err)
	}

	return &pb.GetStoragePolicyResponse{
		StoragePolicy: utils.EntPolicyToProto(policy),
	}, nil
}
func (s *AdminService) UpdatePolicy(ctx context.Context, req *pb.UpdateStoragePolicyRequest) (*pb.GetStoragePolicyResponse, error) {
	storagePolicyClient := s.pc
	id := int32(req.Policy.Id)
	if id == 0 {
		return nil, commonpb.ErrorParamInvalid("policy id cannot be 0")
	}

	sc, tx, ctx, err := data.WithTx(ctx, storagePolicyClient)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create transaction: %w", err)
	}

	_, err = sc.Upsert(ctx, utils.ProtoPolicyToEnt(req.Policy))
	if err != nil {
		_ = data.Rollback(tx)
		return nil, commonpb.ErrorDb("Failed to update policy: %w", err)
	}

	if err := data.Commit(tx); err != nil {
		return nil, commonpb.ErrorDb("Failed to commit transaction: %w", err)
	}

	_ = s.kv.Delete(manager.EntityUrlCacheKeyPrefix)

	getReq := &pb.GetStoragePolicyRequest{Id: id}
	return s.GetPolicy(ctx, getReq)
}
func (s *AdminService) DeletePolicy(ctx context.Context, req *pb.SimpleStoragePolicyRequest) (*emptypb.Empty, error) {
	if req.Id == 1 {
		return nil, filepb.ErrorDeleteDefaultPolicy("default policy cannot be deleted")
	}

	storagePolicyClient := s.pc
	newCtx := context.WithValue(ctx, data.LoadStoragePolicyGroup{}, true)
	newCtx = context.WithValue(newCtx, data.SkipStoragePolicyCache{}, true)
	policy, err := storagePolicyClient.GetPolicyByID(newCtx, int(req.Id))
	if err != nil {
		return nil, filepb.ErrorPolicyNotExist("policy not found")
	}

	used, err := s.fc.IsStoragePolicyUsedByEntities(newCtx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to check policy used by entities: %w", err)
	}

	if used {
		return nil, filepb.ErrorPolicyNotExist("policy is used by entities")
	}

	err = storagePolicyClient.Delete(newCtx, policy)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to delete policy: %w", err)
	}

	return &emptypb.Empty{}, nil
}
func (s *AdminService) CreateStoragePolicyCors(ctx context.Context, req *pb.CreateStoragePolicyCorsRequest) (*pb.GetStoragePolicyResponse, error) {
	storagePolicyClient := s.pc

	if req.Policy.Type == types.PolicyTypeLocal {
		req.Policy.DirNameRule = &wrapperspb.StringValue{Value: util.DataPath("uploads/{uid}/{path}")}
	}

	req.Policy.Id = 0
	policy, err := storagePolicyClient.Upsert(ctx, utils.ProtoPolicyToEnt(req.Policy))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create policy: %w", err)
	}

	return &pb.GetStoragePolicyResponse{
		StoragePolicy: utils.EntPolicyToProto(policy),
	}, nil
}
func (s *AdminService) OdOauthUrl(ctx context.Context, req *pb.GetOauthUrlRequest) (*pb.GetUrlResponse, error) {
	storagePolicyClient := s.pc

	policy, err := storagePolicyClient.GetPolicyByID(ctx, int(req.Id))
	if err != nil || policy.Type != types.PolicyTypeOd {
		return nil, filepb.ErrorPolicyNotExist("policy id %d not exist", req.Id)
	}

	policy.Settings.OauthRedirect = routes.MasterPolicyOAuthCallback(s.settings.SiteURL(ctx)).String()
	policy.SecretKey = req.Secret
	policy.BucketName = req.AppId
	policy, err = storagePolicyClient.Upsert(ctx, policy)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to upsert policy: %w", err)
	}

	client := onedrive.NewClient(policy, s.rc, s.credm, s.l.Logger(), s.settings, 0)
	redirect := client.OAuthURL(context.Background(), []string{
		"offline_access",
		"files.readwrite.all",
	})

	return &pb.GetUrlResponse{Url: redirect}, nil
}
func (s *AdminService) GetPolicyOAuthCallbackUrl(ctx context.Context, req *emptypb.Empty) (*pb.GetUrlResponse, error) {
	uri := routes.MasterPolicyOAuthCallback(s.settings.SiteURL(ctx)).String()
	return &pb.GetUrlResponse{Url: uri}, nil
}
func (s *AdminService) GetStoragePolicyStatus(ctx context.Context, req *pb.SimpleStoragePolicyRequest) (*pb.GetStoragePolicyStatusResponse, error) {
	storagePolicyClient := s.pc

	policy, err := storagePolicyClient.GetPolicyByID(ctx, int(req.Id))
	if err != nil || policy.Type != types.PolicyTypeOd {
		return nil, filepb.ErrorPolicyNotExist("policy id %d not exist", req.Id)
	}

	if policy.AccessKey == "" {
		return &pb.GetStoragePolicyStatusResponse{Valid: false}, nil
	}

	token, err := s.credm.Obtain(ctx, onedrive.CredentialKey(policy.ID))
	if err != nil {
		if errors.Is(err, credmanager.ErrorNotFound) {
			return &pb.GetStoragePolicyStatusResponse{Valid: false}, nil
		}

		return nil, commonpb.ErrorDb("Failed to get credential: %w", err)
	}

	return &pb.GetStoragePolicyStatusResponse{
		Valid:           true,
		LastRefreshTime: timestamppb.New(*token.RefreshedAt()),
	}, nil
}
func (s *AdminService) FinishOAuthCallback(ctx context.Context, req *pb.FinishOAuthCallbackRequest) (*emptypb.Empty, error) {
	storagePolicyClient := s.pc

	policyId, err := strconv.Atoi(req.State)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Invalid state")
	}

	policy, err := storagePolicyClient.GetPolicyByID(ctx, policyId)
	if err != nil {
		return nil, filepb.ErrorPolicyNotExist("policy id %d not exist", policyId)
	}

	client := onedrive.NewClient(policy, s.rc, s.credm, s.l.Logger(), s.settings, 0)
	credential, err := client.ObtainToken(ctx, onedrive.WithCode(req.Code))
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Failed to obtain token: %w", err)
	}

	credManager := s.credm
	if err := credManager.Upsert(ctx, credential); err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to upsert credential")
	}

	if _, err := credManager.Obtain(ctx, onedrive.CredentialKey(policy.ID)); err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to obtain credential")
	}

	return &emptypb.Empty{}, nil
}
func (s *AdminService) GetSharePointDriverRoot(ctx context.Context, req *pb.GetShareDriverRootRequest) (*pb.GetUrlResponse, error) {
	storagePolicyClient := s.pc

	policy, err := storagePolicyClient.GetPolicyByID(ctx, int(req.Id))
	if err != nil {
		return nil, filepb.ErrorPolicyNotExist("policy id %d not exist", req.Id)
	}

	if policy.Type != types.PolicyTypeOd {
		return nil, commonpb.ErrorParamInvalid("Invalid policy type")
	}

	client := onedrive.NewClient(policy, s.rc, s.credm, s.l.Logger(), s.settings, 0)
	root, err := client.GetSiteIDByURL(ctx, req.Url)
	if err != nil {
		return nil, commonpb.ErrorParamInvalid("Failed to get site ID: %w", err)
	}

	return &pb.GetUrlResponse{Url: fmt.Sprintf("sites/%s/drive", root)}, nil
}
