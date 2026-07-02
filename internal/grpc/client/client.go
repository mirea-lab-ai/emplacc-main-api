package client

import (
	"context"
	"fmt"
	"log"
	"time"

	v1 "emplacc-api/internal/grpc/gen/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// LLMOverrides — параметры, которые переопределяют дефолты LLM-сервиса
type LLMOverrides struct {
	Model        string
	URL          string
	SystemPrompt string
	// Mode selects a server-side prompt mode (reports_llm_ms meta["mode"]):
	// report_enrichment | forum_digest | evidence_summary | criteria_suggestion.
	Mode string
}

type LLMClient struct {
	conn   *grpc.ClientConn
	client v1.MCPServiceClient
}

// bearerPerRPC attaches `authorization: Bearer <token>` to every gRPC call so the
// reports_llm_ms server (when GRPC_AUTH_TOKEN is set there) accepts it. Transport
// security is not required because the LLM link is plaintext on the internal
// network; switch to TLS creds if that changes.
type bearerPerRPC struct {
	token string
}

func (b bearerPerRPC) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + b.token}, nil
}

func (b bearerPerRPC) RequireTransportSecurity() bool { return false }

// NewLLMClient dials the LLM gRPC service. When authToken is non-empty it is sent
// as a Bearer token on every call (matches reports_llm_ms GRPC_AUTH_TOKEN).
func NewLLMClient(addr string, authToken string) (*LLMClient, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithTimeout(10 * time.Second),
	}
	if authToken != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(bearerPerRPC{token: authToken}))
	}
	conn, err := grpc.Dial(addr, opts...)
	if err != nil {
		return nil, err
	}
	return &LLMClient{conn: conn, client: v1.NewMCPServiceClient(conn)}, nil
}

func (c *LLMClient) Close() error { return c.conn.Close() }

// buildMeta строит map для передачи оверрайдов в LLM-сервис
func buildMeta(overrides *LLMOverrides) map[string]string {
	meta := map[string]string{}
	if overrides == nil {
		return meta
	}
	if overrides.Model != "" {
		meta["model"] = overrides.Model
	}
	if overrides.URL != "" {
		meta["url"] = overrides.URL
	}
	if overrides.SystemPrompt != "" {
		meta["system_prompt"] = overrides.SystemPrompt
	}
	if overrides.Mode != "" {
		meta["mode"] = overrides.Mode
	}
	return meta
}

// ProcessTaskWithLLM — синхронный вызов с опциональными оверрайдами настроек
func (c *LLMClient) ProcessTaskWithLLM(ctx context.Context, taskDescription, userText, taskId string, overrides *LLMOverrides) (string, error) {
	req := &v1.ProcessTaskRequest{
		Description: taskDescription,
		Text:        userText,
		TaskId:      taskId,
		ContentType: "text/plain",
		TimeoutMs:   120000,
		Meta:        buildMeta(overrides),
	}

	resp, err := c.client.ProcessTask(ctx, req)
	if err != nil {
		log.Printf("gRPC call failed: %v", err)
		return "", err
	}
	return resp.Result, nil
}

// ProcessTaskWithLLMStream — потоковый вызов с опциональными оверрайдами
func (c *LLMClient) ProcessTaskWithLLMStream(ctx context.Context, taskDescription, userText, taskId string, overrides *LLMOverrides) (string, error) {
	req := &v1.ProcessTaskRequest{
		Description: taskDescription,
		Text:        userText,
		TaskId:      taskId,
		ContentType: "text/plain",
		TimeoutMs:   120000,
		Meta:        buildMeta(overrides),
	}

	stream, err := c.client.StreamProcessTask(ctx, req)
	if err != nil {
		log.Printf("gRPC stream call failed: %v", err)
		return "", err
	}

	var result string
	for {
		event, err := stream.Recv()
		if err != nil {
			return "", err
		}
		switch e := event.Event.(type) {
		case *v1.ProcessTaskEvent_Chunk:
			result += e.Chunk.Data
		case *v1.ProcessTaskEvent_Final:
			return e.Final.Result, nil
		case *v1.ProcessTaskEvent_Status:
			log.Printf("LLM status: %s %s", e.Status.State, e.Status.Message)
		case *v1.ProcessTaskEvent_Error:
			return "", fmt.Errorf("LLM error %d: %s", e.Error.Code, e.Error.Message)
		}
	}
}
