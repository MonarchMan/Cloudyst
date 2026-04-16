package service

import (
	filepb "api/api/file/common/v1"
	userpb "api/api/user/common/v1"
	pb "api/api/user/site/v1"
	ftypes "api/external/data/file"
	"common/cache"
	"common/constants"
	"common/hashid"
	"context"
	"sort"
	"strings"
	"user/app/middleware"
	"user/internal/biz/setting"
	"user/internal/data"
	"user/internal/pkg/captcha"

	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	SummaryRangeDays = 12
	MetricCacheKey   = "admin_summary"
	metricErrMsg     = "Failed to generate metrics summary"
)

type SiteService struct {
	pb.UnimplementedSiteServer
	settings setting.Provider
	captcha  captcha.CaptchaHelper
	hasher   hashid.Encoder
	kv       cache.Driver
	uc       data.UserClient
}

func NewSiteService(setting setting.Provider, captcha captcha.CaptchaHelper, hasher hashid.Encoder, kv cache.Driver,
	uc data.UserClient) *SiteService {
	return &SiteService{
		settings: setting,
		captcha:  captcha,
		hasher:   hasher,
		kv:       kv,
		uc:       uc,
	}
}

func (s *SiteService) Ping(ctx context.Context, req *emptypb.Empty) (*pb.PingResponse, error) {
	return &pb.PingResponse{
		Message: constants.BackendVersion,
	}, nil
}
func (s *SiteService) GetCaptcha(ctx context.Context, req *emptypb.Empty) (*pb.CaptchaResponse, error) {
	id, b64s, _, err := s.captcha.GetCaptcha().Generate()
	if err != nil {
		return nil, userpb.ErrorCaptcha("generate captcha failed: %v", err)
	}
	return &pb.CaptchaResponse{
		Image:  b64s,
		Ticket: id,
	}, nil
}
func (s *SiteService) GetSiteConfig(ctx context.Context, req *pb.GetSiteConfigRequest) (*pb.GetSiteConfigResponse, error) {
	switch req.Section {
	case "login":
		legalDocs := s.settings.LegalDocuments(ctx)
		return &pb.GetSiteConfigResponse{
			LoginCaptcha:     s.settings.LoginCaptchaEnabled(ctx),
			RegCaptcha:       s.settings.RegCaptchaEnabled(ctx),
			ForgetCaptcha:    s.settings.ForgotPasswordCaptchaEnabled(ctx),
			Authn:            s.settings.AuthnEnabled(ctx),
			RegisterEnabled:  s.settings.RegisterEnabled(ctx),
			PrivacyPolicyUrl: legalDocs.PrivacyPolicy,
			TosUrl:           legalDocs.TermsOfService,
		}, nil
	case "explorer":
		explorerSettings := s.settings.ExplorerFrontendSettings(ctx)
		mapSettings := s.settings.MapSetting(ctx)
		fileViewers := s.settings.FileViewers(ctx)
		customProps := s.settings.CustomProps(ctx)
		maxBatchSize := s.settings.MaxBatchedFile(ctx)
		w, h := s.settings.ThumbSize(ctx)
		for i := range fileViewers {
			for j := range fileViewers[i].Viewers {
				fileViewers[i].Viewers[j].WopiActions = nil
			}
		}
		return &pb.GetSiteConfigResponse{
			MaxBatchSize: int32(maxBatchSize),
			FileViewers: lo.Map(fileViewers, func(item *ftypes.ViewerGroup, index int) *filepb.ViewerGroup {
				return ftypes.ToProtoViewerGroup(item)
			}),
			Icons:             explorerSettings.Icons,
			MapProvider:       mapSettings.Provider,
			GoogleMapTileType: mapSettings.GoogleTileType,
			MapboxAk:          mapSettings.MapboxAK,
			ThumbnailWidth:    int32(w),
			ThumbnailHeight:   int32(h),
			CustomProps:       customProps,
		}, nil
	case "emojis":
		emojis := s.settings.EmojiPresets(ctx)
		return &pb.GetSiteConfigResponse{
			EmojiPreset: emojis,
		}, nil
	case "app":
		appSetting := s.settings.AppSetting(ctx)
		return &pb.GetSiteConfigResponse{
			AppPromotion: appSetting.Promotion,
		}, nil
	case "thumb":
		// Return supported thumbnail extensions from enabled generators.
		exts := map[string]bool{}
		if s.settings.BuiltinThumbGeneratorEnabled(ctx) {
			for _, e := range constants.SupportedExts {
				exts[e] = true
			}
		}
		if s.settings.FFMpegThumbGeneratorEnabled(ctx) {
			for _, e := range s.settings.FFMpegThumbExts(ctx) {
				exts[strings.ToLower(e)] = true
			}
		}
		if s.settings.VipsThumbGeneratorEnabled(ctx) {
			for _, e := range s.settings.VipsThumbExts(ctx) {
				exts[strings.ToLower(e)] = true
			}
		}
		if s.settings.LibreOfficeThumbGeneratorEnabled(ctx) {
			for _, e := range s.settings.LibreOfficeThumbExts(ctx) {
				exts[strings.ToLower(e)] = true
			}
		}
		if s.settings.MusicCoverThumbGeneratorEnabled(ctx) {
			for _, e := range s.settings.MusicCoverThumbExts(ctx) {
				exts[strings.ToLower(e)] = true
			}
		}
		if s.settings.LibRawThumbGeneratorEnabled(ctx) {
			for _, e := range s.settings.LibRawThumbExts(ctx) {
				exts[strings.ToLower(e)] = true
			}
		}

		// map -> sorted slice
		result := make([]string, 0, len(exts))
		for e := range exts {
			result = append(result, e)
		}
		sort.Strings(result)
		return &pb.GetSiteConfigResponse{ThumbExts: result}, nil
	default:
		break
	}

	u := middleware.UserFromContext(ctx)
	siteBasic := s.settings.SiteBasic(ctx)
	themes := s.settings.Theme(ctx)
	userRes := buildUser(u, s.hasher)
	logo := s.settings.Logo(ctx)
	reCaptcha := s.settings.ReCaptcha(ctx)
	capCaptcha := s.settings.CapCaptcha(ctx)
	appSetting := s.settings.AppSetting(ctx)
	customNavItems := s.settings.CustomNavItems(ctx)
	customHTML := s.settings.CustomHTML(ctx)
	return &pb.GetSiteConfigResponse{
		InstanceId:      siteBasic.ID,
		SiteName:        siteBasic.Name,
		Themes:          themes.Themes,
		DefaultTheme:    themes.DefaultTheme,
		User:            userRes,
		Logo:            logo.Normal,
		LogoLight:       logo.Light,
		CaptchaType:     s.settings.CaptchaType(ctx),
		TurnstileSiteId: s.settings.TurnstileCaptcha(ctx).Key,
		ReCaptchaKey:    reCaptcha.Key,
		CapInstanceUrl:  capCaptcha.InstanceURL,
		CapSiteKey:      capCaptcha.SiteKey,
		CapAssetServer:  capCaptcha.AssetServer,
		AppPromotion:    appSetting.Promotion,
		CustomNavItems:  customNavItems,
		CustomHtml:      customHTML,
	}, nil
}

func (s *SiteService) GetSiteBasicInfo(ctx context.Context, req *emptypb.Empty) (*pb.SettingResponse, error) {
	siteBasic := s.settings.SiteBasic(ctx)
	pwaOpts := s.settings.PWA(ctx)
	theme := s.settings.Theme(ctx)
	return &pb.SettingResponse{
		Settings: map[string]string{
			"site_name":           siteBasic.Name,
			"site_des":            siteBasic.Description,
			"siteScript":          siteBasic.Script,
			"pwa_small_icon":      pwaOpts.SmallIcon,
			"pwa_medium_icon":     pwaOpts.MediumIcon,
			"pwa_large_icon":      pwaOpts.LargeIcon,
			"pwa_display":         pwaOpts.Display,
			"theme_color":         pwaOpts.ThemeColor,
			"default_theme_color": theme.DefaultTheme,
		},
	}, nil
}
