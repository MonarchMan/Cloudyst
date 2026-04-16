package conf

import (
	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/log"
)

type WatchFn func(string2 string, observer config.Observer)

type ConfigWatcher interface {
	Key() string
	Observer() config.Observer
}

func NewConfigWatcher(key string, observer config.Observer) ConfigWatcher {
	return &configWatcher{
		key:      key,
		observer: observer,
	}
}

type configWatcher struct {
	key      string
	observer config.Observer
}

func (w *configWatcher) Key() string {
	return w.key
}

func (w *configWatcher) Observer() config.Observer {
	return w.observer
}

func AddWatcher(c config.Config, watchers ...ConfigWatcher) error {
	for _, watcher := range watchers {
		err := c.Watch(watcher.Key(), watcher.Observer())
		if err != nil {
			return err
		}
	}
	return nil
}

func SetTimeout(key string, value config.Value) {
	log.Infof("config timeout changed: %s = %v", key, value)
	
}
