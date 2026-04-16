package service

import (
	commonpb "api/api/common/v1"
	userpb "api/api/user/common/v1"
	"common/boolset"
	"common/cache"
	"common/constants"
	"common/hashid"
	"common/util"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
	"user/ent"
	"user/ent/user"
	"user/internal/biz/email"
	"user/internal/biz/setting"
	"user/internal/data"
	"user/internal/data/rpc"
	"user/internal/data/types"
	"user/internal/pkg/utils"

	pb "api/api/user/admin/v1"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AdminService struct {
	pb.UnimplementedAdminServer
	kv            cache.Driver
	userClient    data.UserClient
	groupClient   data.GroupClient
	settingClient data.SettingClient
	settings      setting.Provider

	sysClient   rpc.FileSysClient
	fileClient  rpc.FileClient
	shareClient rpc.ShareClient
	taskClient  rpc.FileAdminClient

	hasher hashid.Encoder
	em     email.DriverManager
}

func NewAdminService(kv cache.Driver, uc data.UserClient, gc data.GroupClient, sc data.SettingClient, settings setting.Provider,
	sysC rpc.FileSysClient, fc rpc.FileClient, shareC rpc.ShareClient, tc rpc.FileAdminClient, hasher hashid.Encoder,
	em email.DriverManager) *AdminService {
	return &AdminService{
		kv:            kv,
		userClient:    uc,
		groupClient:   gc,
		settingClient: sc,
		settings:      settings,
		fileClient:    fc,
		sysClient:     sysC,
		shareClient:   shareC,
		taskClient:    tc,
		hasher:        hasher,
		em:            em,
	}
}

func (s *AdminService) AdminListUsers(ctx context.Context, req *userpb.ListRequest) (*pb.ListUserResponse, error) {
	newCtx := context.WithValue(ctx, data.LoadUserGroup{}, true)
	newCtx = context.WithValue(newCtx, data.LoadUserPasskey{}, false)

	var (
		err     error
		groupID int
	)
	if req.Conditions[userGroupCondition] != "" {
		groupID, err = strconv.Atoi(req.Conditions[userGroupCondition])
		if err != nil {
			return nil, commonpb.ErrorParamInvalid("Invalid group ID: %w", err)
		}
	}

	res, err := s.userClient.ListUsers(newCtx, &data.ListUserParameters{
		GroupID: groupID,
		PaginationArgs: &data.PaginationArgs{
			Page:     int(req.Page) - 1,
			PageSize: int(req.PageSize),
			OrderBy:  req.OrderBy,
			Order:    data.OrderDirection(req.OrderDirection),
		},
		Status: user.Status(req.Conditions[userStatusCondition]),
		Nick:   req.Conditions[userNickCondition],
		Email:  req.Conditions[userEmailCondition],
	})
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list users: %w", err)
	}

	return &pb.ListUserResponse{
		Users: lo.Map(res.Users, func(item *ent.User, index int) *pb.AdminGetUserResponse {
			return &pb.AdminGetUserResponse{
				User:         utils.EntUserToProto(item),
				HashId:       hashid.EncodeUserID(s.hasher, item.ID),
				TwoFaEnabled: item.TwoFactorSecret != "",
			}
		}),
		Pagination: res.PaginationResults,
	}, nil
}
func (s *AdminService) AdminGetUser(ctx context.Context, req *pb.SimpleUserRequest) (*pb.AdminGetUserResponse, error) {
	newCtx := context.WithValue(ctx, data.LoadUserGroup{}, true)
	newCtx = context.WithValue(newCtx, data.LoadUserPasskey{}, false)

	u, err := s.userClient.GetByID(newCtx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get users: %w", err)
	}

	return &pb.AdminGetUserResponse{
		User:         utils.EntUserToProto(u),
		HashId:       hashid.EncodeUserID(s.hasher, u.ID),
		TwoFaEnabled: u.TwoFactorSecret != "",
	}, nil
}
func (s *AdminService) AdminCreateUser(ctx context.Context, req *pb.UpsertUserRequest) (*pb.AdminGetUserResponse, error) {
	if req.Password == "" {
		return nil, commonpb.ErrorParamInvalid("Password is required")
	}

	if req.User.Group.Id != 0 {
		return nil, commonpb.ErrorParamInvalid("Group ID must be 0")
	}

	u, err := s.userClient.Upsert(ctx, utils.ProtoUserToEnt(req.User), req.Password, req.TwoFa)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create users: %w", err)
	}

	newReq := &pb.SimpleUserRequest{Id: int32(u.ID)}
	return s.AdminGetUser(ctx, newReq)
}
func (s *AdminService) AdminUpdateUser(ctx context.Context, req *pb.UpsertUserRequest) (*pb.AdminGetUserResponse, error) {
	newCtx := context.WithValue(ctx, data.LoadUserGroup{}, true)
	existing, err := s.userClient.GetByID(newCtx, int(req.User.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get users: %w", err)
	}

	if req.User.Id == 1 && existing.Edges.Group.Permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		if int(req.User.GroupUsers) != existing.GroupUsers {
			return nil, userpb.ErrorInvalidActionOnDefaultUser("Cannot change default users' group")
		}

		if req.User.Status != utils.GetUserStatus(user.StatusActive) {
			return nil, userpb.ErrorInvalidActionOnDefaultUser("Cannot change default users' status")
		}
	}

	if req.Password != "" && len(req.Password) > 128 {
		return nil, commonpb.ErrorParamInvalid("Password too long")
	}

	newUser, err := s.userClient.Upsert(newCtx, utils.ProtoUserToEnt(req.User), req.Password, req.TwoFa)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update users: %w", err)
	}

	newReq := &pb.SimpleUserRequest{Id: int32(newUser.ID)}
	return s.AdminGetUser(ctx, newReq)
}
func (s *AdminService) AdminDeleteUsers(ctx context.Context, req *pb.DeleteUsersRequest) (*emptypb.Empty, error) {
	curUser := data.UserFromContext(ctx)

	uc, tx, ctx, err := data.WithTx(ctx, s.userClient)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}

	uids := lo.Map(req.Ids, func(id int32, index int) int {
		return int(id)
	})
	uids = lo.Filter(uids, func(id int, index int) bool {
		return id != curUser.ID && id != 1
	})
	if _, err := uc.BatchDelete(ctx, uids); err != nil {
		_ = data.Rollback(tx)
		return nil, fmt.Errorf("failed to delete users: %w", err)
	}

	if err := data.Commit(tx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// 删除用户文件
	err = s.fileClient.DeleteTasksByUser(ctx, req.Ids...)
	if err != nil {
		return nil, err
	}

	// 删除用户任务
	err = s.taskClient.DeleteTasksByUser(ctx, req.Ids...)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to delete tasks: %w", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *AdminService) GetSummary(ctx context.Context, req *pb.GetSummaryRequest) (*pb.GetSummaryResponse, error) {
	res := &pb.GetSummaryResponse{
		Version: &pb.Version{
			Version: constants.BackendVersion,
			//Commit:  constants.las,
		},
		SiteUrls: lo.Map(s.settings.AllSiteURLs(ctx), func(item *url.URL, index int) string {
			return item.String()
		}),
	}

	if summary, ok := s.kv.Get(MetricCacheKey); ok {
		res.MetricsSummary = summary.(*pb.MetricsSummary)
		return res, nil
	}

	summary := &pb.MetricsSummary{
		Dates:       make([]*timestamppb.Timestamp, SummaryRangeDays),
		Files:       make([]int32, SummaryRangeDays),
		Users:       make([]int32, SummaryRangeDays),
		Shares:      make([]int32, SummaryRangeDays),
		GeneratedAt: timestamppb.New(time.Now()),
	}

	toRound := time.Now()
	timeBase := time.Date(toRound.Year(), toRound.Month(), toRound.Day()+1, 0, 0, 0, 0, toRound.Location())
	for day := range summary.Files {
		start := timeBase.Add(-time.Duration(SummaryRangeDays-day) * time.Hour * 24)
		end := timeBase.Add(-time.Duration(SummaryRangeDays-day-1) * time.Hour * 24)
		summary.Dates[day] = timestamppb.New(start)
		fileTotal, err := s.fileClient.CountByTimeRange(ctx, start, end)
		if err != nil {
			return nil, commonpb.ErrorDb(metricErrMsg)
		}
		userTotal, err := s.userClient.CountByTimeRange(ctx, start, end)
		if err != nil {
			return nil, commonpb.ErrorDb(metricErrMsg)
		}
		shareTotal, err := s.shareClient.CountByTimeRange(ctx, start, end)
		if err != nil {
			return nil, commonpb.ErrorDb(metricErrMsg)
		}
		summary.Files[day] = fileTotal
		summary.Users[day] = int32(userTotal)
		summary.Shares[day] = shareTotal
	}

	var err error
	fNum, err := s.fileClient.CountByTimeRange(ctx, time.Time{}, time.Time{})
	if err != nil {
		return nil, commonpb.ErrorDb(metricErrMsg)
	}
	summary.FileTotal = fNum
	ur, err := s.userClient.CountByTimeRange(ctx, time.Time{}, time.Time{})
	if err != nil {
		return nil, commonpb.ErrorDb(metricErrMsg)
	}
	summary.UserTotal = int32(ur)
	sNum, err := s.shareClient.CountByTimeRange(ctx, time.Time{}, time.Time{})
	if err != nil {
		return nil, commonpb.ErrorDb(metricErrMsg)
	}
	summary.ShareTotal = sNum

	_ = s.kv.Set(MetricCacheKey, summary, 86400)
	res.MetricsSummary = summary

	return res, nil
}

func (s *AdminService) GetSettings(ctx context.Context, req *pb.AdminGetSettingRequest) (*pb.AdminSettingResponse, error) {
	res, err := s.settingClient.Gets(ctx, lo.Filter(req.Keys, func(item string, index int) bool {
		return item != "secret_key"
	}))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get settings: %w", err)
	}

	return &pb.AdminSettingResponse{
		Settings: res,
	}, nil
}
func (s *AdminService) SetSettings(ctx context.Context, req *pb.AdminSetSettingRequest) (*pb.AdminSettingResponse, error) {
	// Preprocess settings
	changeEmailCLient := false
	ToForwardKeys := make([]string, 0)
	for k, _ := range req.Settings {
		if preprocessor, ok := preprocessors[k]; ok {
			if err := preprocessor(ctx, req.Settings); err != nil {
				return nil, err
			}
		}

		if lo.Contains(emailPostprocessKeys, k) {
			changeEmailCLient = true
		} else if strings.HasPrefix(k, "file_") {
			k = strings.Replace(k, "file_", "", 1)
			ToForwardKeys = append(ToForwardKeys, k)
		}
	}

	// Save to db
	sc, tx, ctx, err := data.WithTx(ctx, s.settingClient)
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create transaction: %w", err)
	}

	if err := sc.Set(ctx, req.Settings); err != nil {
		_ = data.Rollback(tx)
		return nil, commonpb.ErrorDb("Failed to save settings: %w", err)
	}

	if err := data.Commit(tx); err != nil {
		return nil, commonpb.ErrorDb("Failed to commit transaction: %w", err)
	}

	// Clean cache
	if err := s.kv.Delete(setting.KvSettingPrefix, lo.Keys(req.Settings)...); err != nil {
		return nil, commonpb.ErrorInternalSetting("Failed to clear cache: %w", err)
	}

	if changeEmailCLient {
		s.em.Reload(ctx)
	}

	if len(ToForwardKeys) > 0 {
		err := s.sysClient.ReloadDependency(context.Background(), ToForwardKeys)
		if err != nil {
			return nil, commonpb.ErrorDb("Failed to post process file settings: %w", err)
		}
	}

	return &pb.AdminSettingResponse{
		Settings: req.Settings,
	}, nil
}

func (s *AdminService) AdminListGroups(ctx context.Context, req *userpb.ListRequest) (*pb.AdminListGroupResponse, error) {
	res, err := s.groupClient.ListGroups(ctx, &data.ListGroupParameters{
		PaginationArgs: &data.PaginationArgs{
			Page:     int(req.Page) - 1,
			PageSize: int(req.PageSize),
			OrderBy:  req.OrderBy,
			Order:    data.OrderDirection(req.OrderDirection),
		},
	})
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to list groups: %w", err)
	}

	return &pb.AdminListGroupResponse{
		Groups: lo.Map(res.Groups, func(item *ent.Group, index int) *userpb.Group {
			return utils.EntGroupToProto(item)
		}),
		Pagination: res.PaginationResults,
	}, nil
}
func (s *AdminService) AdminGetGroup(ctx context.Context, req *pb.GetGroupRequest) (*pb.AdminGetGroupResponse, error) {
	group, err := s.groupClient.GetByID(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to get group: %w", err)
	}

	res := &pb.AdminGetGroupResponse{Group: utils.EntGroupToProto(group)}
	if req.CountUser {
		totalUsers, err := s.groupClient.CountUsers(ctx, int(req.Id))
		if err != nil {
			return nil, commonpb.ErrorDb("Failed to count users: %w", err)
		}
		res.TotalUsers = int32(totalUsers)
	}
	return res, nil
}
func (s *AdminService) AdminCreateGroup(ctx context.Context, req *pb.UpsertGroupRequest) (*pb.AdminGetGroupResponse, error) {
	if req.Group.Id > 0 {
		return nil, commonpb.ErrorParamInvalid("ID must be 0")
	}

	group, err := s.groupClient.Upsert(ctx, utils.ProtoGroupToEnt(req.Group))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to create group: %w", err)
	}

	newReq := &pb.GetGroupRequest{Id: int32(group.ID)}
	return s.AdminGetGroup(ctx, newReq)
}
func (s *AdminService) AdminUpdateGroup(ctx context.Context, req *pb.UpsertGroupRequest) (*pb.AdminGetGroupResponse, error) {
	if req.Group.Id == 0 {
		return nil, commonpb.ErrorParamInvalid("ID is required")
	}

	permissions := boolset.BooleanSet(req.Group.Permissions)
	if req.Group.Id == 1 && !permissions.Enabled(int(types.GroupPermissionIsAdmin)) {
		return nil, commonpb.ErrorParamInvalid("Initial admin group have to be admin")
	}

	group, err := s.groupClient.Upsert(ctx, utils.ProtoGroupToEnt(req.Group))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to update group: %w", err)
	}

	newReq := &pb.GetGroupRequest{Id: int32(group.ID)}
	return s.AdminGetGroup(ctx, newReq)
}
func (s *AdminService) DeleteGroup(ctx context.Context, req *pb.SimpleGroupRequest) (*emptypb.Empty, error) {
	if req.Id <= 3 {
		return nil, pb.ErrorInvalidActionOnSystemGroup("invalid action")
	}

	users, err := s.groupClient.CountUsers(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to count users: %w", err)
	}

	if users > 0 {
		return nil, pb.ErrorGroupUsedByUser("group is used by %d users", users)
	}

	err = s.groupClient.Delete(ctx, int(req.Id))
	if err != nil {
		return nil, commonpb.ErrorDb("Failed to delete group: %w", err)
	}

	return &emptypb.Empty{}, nil
}

type (
	SettingPreProcessor  func(ctx context.Context, settings map[string]string) error
	SettingPostProcessor func(ctx context.Context) error
)

var (
	preprocessors = map[string]SettingPreProcessor{
		"siteURL":      siteUrlPreProcessor,
		"mime_mapping": mimeMappingPreProcessor,
		"secret_key":   secretKeyPreProcessor,
	}
	emailPostprocessKeys = []string{
		"smtpUser",
		"smtpPass",
		"smtpHost",
		"smtpPort",
		"smtpEncryption",
		"smtpFrom",
		"replyTo",
		"fromName",
		"fromAdress",
		"secret_key",
	}
)

func siteUrlPreProcessor(ctx context.Context, settings map[string]string) error {
	siteURL := settings["siteURL"]
	urls := strings.Split(siteURL, ",")
	for index, u := range urls {
		urlParsed, err := url.Parse(u)
		if err != nil {
			return fmt.Errorf("Failed to parse siteURL %q: %w", u, err)
		}

		urls[index] = urlParsed.String()
	}
	settings["siteURL"] = strings.Join(urls, ",")
	return nil
}

func secretKeyPreProcessor(ctx context.Context, settings map[string]string) error {
	settings["secret_key"] = util.RandStringRunes(256)
	return nil
}

func mimeMappingPreProcessor(ctx context.Context, settings map[string]string) error {
	var mapping map[string]string
	if err := json.Unmarshal([]byte(settings["mime_mapping"]), &mapping); err != nil {
		return commonpb.ErrorParamInvalid("Invalid mime mapping: %w", err)
	}

	return nil
}
