package captcha

import (
	"common"
	"common/cache"
	"context"
	"sync/atomic"
	"time"
	"user/internal/biz/setting"

	"github.com/mojocn/base64Captcha"
)

type (
	CaptchaHelper interface {
		common.Reloadable
		GetCaptcha() *base64Captcha.Captcha
	}
	captchaHelper struct {
		captcha  atomic.Value
		store    base64Captcha.Store
		settings setting.Provider
	}
)

func NewCaptchaHelper(kv cache.Driver, settings setting.Provider) CaptchaHelper {
	return &captchaHelper{
		store:    NewRedisStore(kv, "captcha", time.Minute*5),
		settings: settings,
	}
}

func (h *captchaHelper) Reload(ctx context.Context) error {
	captchaSettings := h.settings.Captcha(ctx)
	var driver base64Captcha.Driver
	switch captchaSettings.Mode {
	case setting.CaptchaModeAudio:
		driver = &base64Captcha.DriverAudio{
			Length:   captchaSettings.Length,
			Language: captchaSettings.Language,
		}
	case setting.CaptchaModeNumberString:
		driver = &base64Captcha.DriverString{
			Height:          captchaSettings.Height,
			Width:           captchaSettings.Width,
			NoiseCount:      captchaSettings.ComplexOfNoiseText,
			ShowLineOptions: captchaSettings.ShowLineOptions,
			Length:          captchaSettings.Length,
			Source:          captchaSettings.Source,
			//BgColor:         captchaSettings.BgColor,
			Fonts: captchaSettings.Fonts,
		}
	case setting.CaptchaModeChinese:
		driver = &base64Captcha.DriverChinese{
			Height:          captchaSettings.Height,
			Width:           captchaSettings.Width,
			NoiseCount:      captchaSettings.ComplexOfNoiseText,
			ShowLineOptions: captchaSettings.ShowLineOptions,
			Length:          captchaSettings.Length,
			Source:          captchaSettings.Source,
			//BgColor:         nil,
			Fonts: captchaSettings.Fonts,
		}
	case setting.CaptchaModeMath:
		driver = &base64Captcha.DriverMath{
			Height:          captchaSettings.Height,
			Width:           captchaSettings.Width,
			NoiseCount:      captchaSettings.ComplexOfNoiseText,
			ShowLineOptions: captchaSettings.ShowLineOptions,
			//BgColor:         nil,
			Fonts: captchaSettings.Fonts,
		}
	default:
		driver = &base64Captcha.DriverDigit{
			Height:   captchaSettings.Height,
			Width:    captchaSettings.Width,
			Length:   captchaSettings.Length,
			MaxSkew:  captchaSettings.MaxSkew,
			DotCount: captchaSettings.ComplexOfNoiseText,
		}
	}

	h.captcha.Store(base64Captcha.NewCaptcha(driver, h.store))
	return nil
}

func (h *captchaHelper) GetCaptcha() *base64Captcha.Captcha {
	if v, ok := h.captcha.Load().(*base64Captcha.Captcha); ok {
		return v
	}
	return nil
}
