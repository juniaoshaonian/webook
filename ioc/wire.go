//go:build wireinject

package ioc

import (
	"github.com/ecodeclub/webook/internal/cos"
	baguwen "github.com/ecodeclub/webook/internal/question"
	"github.com/google/wire"
)

var BaseSet = wire.NewSet(InitDB, InitCache, InitRedis, InitCosConfig)

func InitApp() (*App, error) {
	wire.Build(wire.Struct(new(App), "*"),
		BaseSet,
		cos.InitHandler,
		baguwen.InitHandler,
		baguwen.InitQuestionSetHandler,
		InitUserHandler,
		InitSession,
		initGinxServer)
	return new(App), nil
}
