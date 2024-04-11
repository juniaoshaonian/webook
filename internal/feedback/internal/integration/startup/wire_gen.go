// Code generated by Wire. DO NOT EDIT.

//go:generate go run github.com/google/wire/cmd/wire
//go:build !wireinject
// +build !wireinject

package startup

import (
	"github.com/ecodeclub/webook/internal/feedback"
	"github.com/ecodeclub/webook/internal/feedback/internal/event"
	"github.com/ecodeclub/webook/internal/feedback/internal/web"
	testioc "github.com/ecodeclub/webook/internal/test/ioc"
)

// Injectors from wire.go:

func InitHandler(p event.IncreaseCreditsEventProducer) (*web.Handler, error) {
	db := testioc.InitDB()
	service := feedback.InitService(db, p)
	handler := web.NewHandler(service)
	return handler, nil
}