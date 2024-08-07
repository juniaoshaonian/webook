package zhipu

import (
	"context"
	"math"

	"github.com/ecodeclub/webook/internal/ai/internal/domain"
	"github.com/yankeguo/zhipu"
)

// Handler 如果后续有不同的实现，就提供不同的实现
type Handler struct {
	client *zhipu.Client
	svc    *zhipu.ChatCompletionService
	// 价格和 model 进行绑定的
	price float64
}

func NewHandler(apikey string,
	price float64) (*Handler, error) {
	client, err := zhipu.NewClient(zhipu.WithAPIKey(apikey))
	if err != nil {
		return nil, err
	}
	const model = "glm-4-0520"
	svc := client.ChatCompletion(model)
	return &Handler{
		client: client,
		// 后续可以做成可配置的
		svc:   svc,
		price: price,
	}, err
}

func (h *Handler) Name() string {
	return "zhipu"
}

func (h *Handler) Handle(ctx context.Context, req domain.LLMRequest) (domain.LLMResponse, error) {
	// 这边它不会调用 next，因为它是最终的出口
	completion, err := h.svc.AddTool(zhipu.ChatCompletionToolRetrieval{
		KnowledgeID: req.Config.KnowledgeId,
	}).AddMessage(zhipu.ChatCompletionMessage{
		Role:    "user",
		Content: req.Prompt,
	}).Do(ctx)
	if err != nil {
		return domain.LLMResponse{}, err
	}
	tokens := completion.Usage.TotalTokens
	// 现在的报价都是 N/1k token
	// 而后向上取整
	amt := math.Ceil(float64(tokens) * h.price / 1000)
	// 金额只有具体的模型才知道怎么算
	resp := domain.LLMResponse{
		Tokens: tokens,
		Amount: int64(amt),
	}

	if len(completion.Choices) > 0 {
		resp.Answer = completion.Choices[0].Message.Content
	}
	return resp, nil
}
