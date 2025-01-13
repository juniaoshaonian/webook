// Copyright 2023 ecodeclub
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build e2e

package integration

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/ecodeclub/webook/internal/ai"

	"github.com/ecodeclub/webook/internal/permission"
	permissionmocks "github.com/ecodeclub/webook/internal/permission/mocks"
	"github.com/ecodeclub/webook/internal/question/internal/errs"

	"github.com/ecodeclub/webook/internal/interactive"
	intrmocks "github.com/ecodeclub/webook/internal/interactive/mocks"

	eveMocks "github.com/ecodeclub/webook/internal/question/internal/event/mocks"
	"go.uber.org/mock/gomock"

	"github.com/ecodeclub/webook/internal/question/internal/domain"

	"github.com/ecodeclub/webook/internal/pkg/middleware"

	"github.com/ecodeclub/ecache"
	"github.com/ecodeclub/ekit/iox"
	"github.com/ecodeclub/ginx/session"
	"github.com/ecodeclub/webook/internal/question/internal/integration/startup"
	"github.com/ecodeclub/webook/internal/question/internal/repository/dao"
	"github.com/ecodeclub/webook/internal/question/internal/web"
	"github.com/ecodeclub/webook/internal/test"
	testioc "github.com/ecodeclub/webook/internal/test/ioc"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/econf"
	"github.com/gotomicro/ego/server/egin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const uid = 123

type HandlerTestSuite struct {
	BaseTestSuite
	server *egin.Component
	rdb    ecache.Cache
}

func (s *HandlerTestSuite) SetupSuite() {
	ctrl := gomock.NewController(s.T())
	producer := eveMocks.NewMockSyncEventProducer(ctrl)

	intrSvc := intrmocks.NewMockService(ctrl)
	intrModule := &interactive.Module{
		Svc: intrSvc,
	}

	// 模拟返回的数据
	// 使用如下规律:
	// 1. liked == id % 2 == 1 (奇数为 true)
	// 2. collected = id %2 == 0 (偶数为 true)
	// 3. viewCnt = id + 1
	// 4. likeCnt = id + 2
	// 5. collectCnt = id + 3
	intrSvc.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		AnyTimes().DoAndReturn(func(ctx context.Context,
		biz string, id int64, uid int64) (interactive.Interactive, error) {
		intr := s.mockInteractive(biz, id)
		return intr, nil
	})
	intrSvc.EXPECT().GetByIds(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context,
		biz string, ids []int64) (map[int64]interactive.Interactive, error) {
		res := make(map[int64]interactive.Interactive, len(ids))
		for _, id := range ids {
			intr := s.mockInteractive(biz, id)
			res[id] = intr
		}
		return res, nil
	}).AnyTimes()

	permSvc := permissionmocks.NewMockService(ctrl)
	// biz id 为偶数就有权限
	permSvc.EXPECT().HasPermission(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context,
		perm permission.Permission) (bool, error) {
		return perm.BizID%2 == 0, nil
	}).AnyTimes()

	module, err := startup.InitModule(producer, nil, intrModule,
		&permission.Module{Svc: permSvc}, &ai.Module{})
	require.NoError(s.T(), err)
	econf.Set("server", map[string]any{"contextTimeout": "1s"})
	server := egin.Load("server").Build()

	module.Hdl.PublicRoutes(server.Engine)
	module.QsHdl.PublicRoutes(server.Engine)
	server.Use(func(ctx *gin.Context) {
		notMember := ctx.GetHeader("not_member") == "1"

		data := map[string]string{
			"creator": "true",
		}

		// 如果不是会员,添加memberDDL
		if !notMember {
			data["memberDDL"] = strconv.FormatInt(time.Now().Add(time.Hour).UnixMilli(), 10)
		}

		ctx.Set("_session", session.NewMemorySession(session.Claims{
			Uid:  uid,
			Data: data,
		}))
	})
	module.QsHdl.PrivateRoutes(server.Engine)
	server.Use(middleware.NewCheckMembershipMiddlewareBuilder(nil).Build())
	module.Hdl.MemberRoutes(server.Engine)

	s.server = server
	s.db = testioc.InitDB()
	err = dao.InitTables(s.db)
	require.NoError(s.T(), err)
	s.rdb = testioc.InitCache()
}

func (s *HandlerTestSuite) TestPubList() {
	// 插入一百条
	data := make([]dao.PublishQuestion, 0, 100)
	for idx := 0; idx < 100; idx++ {
		id := int64(idx + 1)
		data = append(data, dao.PublishQuestion{
			Id:      id,
			Uid:     uid,
			Biz:     domain.DefaultBiz,
			BizId:   id,
			Status:  domain.UnPublishedStatus.ToUint8(),
			Title:   fmt.Sprintf("这是标题 %d", idx),
			Content: fmt.Sprintf("这是解析 %d", idx),
			Utime:   123,
		})
	}

	// project 的不会被搜索到
	data = append(data, dao.PublishQuestion{
		Id:      101,
		Uid:     uid,
		Biz:     "project",
		BizId:   101,
		Status:  domain.UnPublishedStatus.ToUint8(),
		Title:   fmt.Sprintf("这是标题 %d", 101),
		Content: fmt.Sprintf("这是解析 %d", 101),
		Utime:   123,
	})

	err := s.db.Create(&data).Error
	require.NoError(s.T(), err)
	testCases := []struct {
		name string
		req  web.Page

		wantCode int
		wantResp test.Result[web.QuestionList]
	}{
		{
			name: "获取成功",
			req: web.Page{
				Limit:  2,
				Offset: 0,
			},
			wantCode: 200,
			wantResp: test.Result[web.QuestionList]{
				Data: web.QuestionList{
					Total: 100,
					Questions: []web.Question{
						{
							Id:      100,
							Title:   "这是标题 99",
							Content: "这是解析 99",
							Status:  domain.UnPublishedStatus.ToUint8(),
							Utime:   123,
							Biz:     domain.DefaultBiz,
							BizId:   100,
							Interactive: web.Interactive{
								ViewCnt:    101,
								LikeCnt:    102,
								CollectCnt: 103,
								Liked:      false,
								Collected:  true,
							},
						},
						{
							Id:      99,
							Title:   "这是标题 98",
							Content: "这是解析 98",
							Status:  domain.UnPublishedStatus.ToUint8(),
							Utime:   123,
							Biz:     domain.DefaultBiz,
							BizId:   99,
							Interactive: web.Interactive{
								ViewCnt:    100,
								LikeCnt:    101,
								CollectCnt: 102,
								Liked:      true,
								Collected:  false,
							},
						},
					},
				},
			},
		},
		{
			name: "获取部分",
			req: web.Page{
				Limit:  2,
				Offset: 99,
			},
			wantCode: 200,
			wantResp: test.Result[web.QuestionList]{
				Data: web.QuestionList{
					Total: 100,
					Questions: []web.Question{
						{
							Id:      1,
							Title:   "这是标题 0",
							Content: "这是解析 0",
							Biz:     domain.DefaultBiz,
							BizId:   1,
							Status:  domain.UnPublishedStatus.ToUint8(),
							Utime:   123,
							Interactive: web.Interactive{
								ViewCnt:    2,
								LikeCnt:    3,
								CollectCnt: 4,
								Liked:      true,
								Collected:  false,
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.T().Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost,
				"/question/list", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[web.QuestionList]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			assert.Equal(t, tc.wantResp, recorder.MustScan())
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = s.rdb.Delete(ctx, "question:total")
	require.NoError(s.T(), err)
}

func (s *HandlerTestSuite) TestPubDetail() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	// 插入一百条
	data := make([]dao.PublishQuestion, 0, 2)
	results := make([]dao.QuestionResult, 0, 2)
	for idx := 0; idx < 3; idx++ {
		id := int64(idx + 1)
		data = append(data, dao.PublishQuestion{
			Id:      id,
			Uid:     uid,
			BizId:   id,
			Biz:     "project",
			Status:  domain.PublishedStatus.ToUint8(),
			Title:   fmt.Sprintf("这是标题 %d", idx),
			Content: fmt.Sprintf("这是解析 %d", idx),
		})

		results = append(results, dao.QuestionResult{
			Uid:    uid,
			Qid:    int64(idx + 1),
			Result: domain.ResultIntermediate.ToUint8(),
		})
	}
	err := s.db.WithContext(ctx).Create(&data).Error
	require.NoError(s.T(), err)
	// 插入对应的评分数据
	s.db.WithContext(ctx).Create(&results)
	testCases := []struct {
		name string

		req      web.Qid
		before   func(r *http.Request) *http.Request
		wantCode int
		wantResp test.Result[web.Question]
	}{
		{
			name: "查询到了数据",
			req: web.Qid{
				Qid: 2,
			},
			wantCode: 200,
			wantResp: test.Result[web.Question]{
				Data: web.Question{
					Id:      2,
					Title:   "这是标题 1",
					Biz:     "project",
					BizId:   2,
					Status:  domain.PublishedStatus.ToUint8(),
					Content: "这是解析 1",
					Utime:   0,
					Interactive: web.Interactive{
						ViewCnt:    3,
						LikeCnt:    4,
						CollectCnt: 5,
						Liked:      false,
						Collected:  true,
					},
					ExamineResult: domain.ResultIntermediate.ToUint8(),
				},
			},
		},
		{
			name: "没有权限",
			req: web.Qid{
				Qid: 3,
			},
			wantCode: 500,
			wantResp: test.Result[web.Question]{
				Msg:  errs.SystemError.Msg,
				Code: errs.SystemError.Code,
			},
		},
	}
	for _, tc := range testCases {
		s.T().Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost,
				"/question/detail", iox.NewJSONReader(tc.req))
			req.Header.Set("content-type", "application/json")
			require.NoError(t, err)
			recorder := test.NewJSONResponseRecorder[web.Question]()
			s.server.ServeHTTP(recorder, req)
			require.Equal(t, tc.wantCode, recorder.Code)
			assert.Equal(t, tc.wantResp, recorder.MustScan())
		})
	}
}

func (s *HandlerTestSuite) TestPubPartDetail() {
	data := make([]dao.PublishQuestion, 0, 2)
	results := make([]dao.QuestionResult, 0, 2)
	que := dao.PublishQuestion{
		Id:      1041,
		Uid:     uid,
		BizId:   0,
		Biz:     "baguwen",
		Status:  domain.PublishedStatus.ToUint8(),
		Ctime:   123,
		Utime:   321,
		Title:   `在微服务架构中，如何处理服务实例的动态变化（如上线、下线、故障）？`,
		Content: `<p>略难的题，一般只会出现在社招中。</p><p></p><p>其实这种问法会让你觉得摸不着头脑，但是如果你把问题换成如果服务实例动态变化了，注册中心和客户端会怎样，就清晰多了。要在这个问题之下刷亮点，赢得竞争优势，你可以讨论客户端容错策略，以及高并发场景下服务实例频繁变化会给注册中心带来庞大的压力这两个点。</p>`,
	}
	analysis := dao.AnswerElement{
		Id:      6110,
		Qid:     1041,
		Type:    1,
		Content: `<p>前置知识：</p><ul><li><a href="https://i.meoying.com/question/detail?id=1036" rel="noopener noreferrer" target="_blank">你知道注册中心吗？</a></li></ul><p></p><p>在服务注册与发现中，服务健康检查是确保服务实例可用性的重要机制。通过健康检查，注册中心可以动态感知服务实例的状态变化（如健康、故障、下线等），从而保障消费者调用的服务始终可用。常见的健康检查方式主要有以下两类：</p><p></p><ol><li>主动健康检查：主动健康检查由注册中心或消费者主动发起探测请求，定期检测服务实例的健康状态。常见实现方式包括：<ul><li>HTTP 检查：注册中心向服务实例的健康检查端点（如 /health）发送 HTTP 请求，根据返回状态码（如 2xx）判断健康状态。<ul><li>优点：简单易用，适合 HTTP 服务。</li><li>缺点：只能检测服务的基本可达性，无法深入检测内部状态。</li><li>示例：Spring Boot Actuator 提供了 /actuator/health 端点，Nacos 和 Consul 支持通过 HTTP 检查服务健康。</li></ul></li><li>TCP 检查：注册中心尝试连接服务实例的指定端口，判断端口是否可用。<ul><li>优点：适合非 HTTP 服务（如数据库、消息队列）。</li><li>缺点：仅能检测端口连通性，无法反映业务逻辑状态。</li><li>示例：Consul 支持通过 TCP 检查服务端口。</li></ul></li><li>gRPC 检查：注册中心调用服务实例的 gRPC 健康检查接口（如 grpc.health.v1.Health/Check），判断服务是否健康。<ul><li>优点：适用于 gRPC 服务，通信高效。</li><li>缺点：需要服务实例实现 gRPC 健康检查接口。</li><li>示例：gRPC 官方提供了健康检查协议，适用于 gRPC 服务。</li></ul></li><li>自定义脚本检查：注册中心通过运行自定义脚本或命令检测服务状态。<ul><li>优点：灵活性高，可根据业务需求定制。</li><li>缺点：实现复杂，可能增加系统开销。</li><li>示例：Consul 支持通过 Shell 脚本实现自定义健康检查。</li></ul></li></ul></li><li>被动健康检查：被动健康检查通过监控服务实例的运行状态或调用结果，间接判断健康状况。常见实现方式包括：<ul><li>心跳检测：服务实例定期向注册中心发送心跳信号。如果在规定时间内未收到心跳，则认为实例不可用。<ul><li>优点：实现简单，适合大规模服务实例监控。</li><li>缺点：无法检测服务内部的业务逻辑状态。</li><li>示例：Eureka 和 Nacos 使用心跳机制维持服务健康状态。</li></ul></li><li>请求失败率监控：注册中心或消费者监控服务实例的请求失败率（如超时、错误响应等），当失败率超过阈值时，将实例标记为不可用。<ul><li>优点：能反映服务的实际运行状态。</li><li>缺点：需要额外的监控逻辑，可能存在延迟。</li><li>示例：Hystrix 和 Sentinel 可基于失败率隔离故障实例。</li></ul></li><li>日志监控：通过分析服务实例的运行日志，检测是否存在异常（如错误日志、超时日志）。<ul><li>优点：能深入了解服务运行状态。</li><li>缺点：实现复杂，实时性较差。</li><li>示例：使用 ELK（Elasticsearch、Logstash、Kibana）分析服务日志。</li></ul></li></ul></li></ol><p></p><p>在实际场景中，单一健康检查方式往往不足以全面反映服务状态，因此通常结合多种方式使用，并通过优化策略提升效率和准确性。例如：</p><ul><li>组合检查：<ul><li>主动 + 被动检查：通过 HTTP 检查服务的基本可达性，同时结合心跳检测判断服务是否仍然活跃。</li><li>多级检查：先通过 TCP 检查端口连通性，再通过 HTTP 检查服务业务逻辑状态。</li></ul></li><li>优化策略：<ul><li>调整检查频率：根据服务的重要性和负载情况，合理设置检查频率，避免过于频繁导致性能开销。</li><li>健康状态缓存：对健康检查结果进行短时间缓存，减少重复检查的开销。</li><li>多次失败判定：避免因短暂网络波动或服务抖动导致误判，可设置连续多次失败后才标记为不可用。</li><li>分布式健康检查：在大规模分布式系统中，将健康检查任务分散到多个节点，降低注册中心的压力。</li></ul></li></ul><p>在复杂场景中，还可以基于以下方式提升健康检查的深度和智能化：</p><ul><li><ul><li>依赖检查：检测服务依赖的资源（如数据库、缓存）是否正常。</li><li>业务指标检查：通过关键业务指标（如订单处理速度）判断服务健康状态。</li><li>AI大模型预测：利用AI大模型分析历史数据，提前预测潜在故障。</li></ul></li></ul><p></p><p>服务健康检查是服务注册与发现的关键环节，常见方式包括主动健康检查（如 HTTP、TCP、gRPC、自定义脚本）和被动健康检查（如心跳检测、失败率监控、日志分析）。主动检查适合检测服务的基本可达性，被动检查更能反映服务的实际运行状态。在实际应用中，通常结合多种方式，并通过优化策略提升健康检查的效率和准确性，从而保障微服务架构的稳定性和可用性。</p>`,
	}

}

func TestHandler(t *testing.T) {
	suite.Run(t, new(HandlerTestSuite))
}
