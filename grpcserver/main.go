package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	greeterpb "grpcserver/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type greeterServer struct {
	greeterpb.UnimplementedGreeterServer
}

func (s *greeterServer) SayHello(ctx context.Context, req *greeterpb.HelloRequest) (*greeterpb.HelloReply, error) {
	name := req.GetName()
	if name == "" {
		name = "world"
	}
	return &greeterpb.HelloReply{Message: "Hello, " + name + "!"}, nil
}

func main() {
	grpcAddr := envOr("GRPC_ADDR", ":50051")
	httpAddr := envOr("HTTP_ADDR", ":8080")

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("gRPC 监听失败 (%s): %v", grpcAddr, err)
	}
	grpcSrv := grpc.NewServer()
	greeterpb.RegisterGreeterServer(grpcSrv, &greeterServer{})

	go func() {
		log.Printf("gRPC 服务已启动: %s", grpcAddr)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("gRPC 服务异常: %v", err)
		}
	}()

	conn, err := grpc.NewClient(dialTarget(grpcAddr), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("连接本地 gRPC 服务失败: %v", err)
	}
	defer conn.Close()

	httpSrv := &http.Server{
		Addr:              httpAddr,
		Handler:           newHTTPHandler(greeterpb.NewGreeterClient(conn)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		log.Printf("HTTP 网关已启动: %s", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP 网关异常: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	log.Println("正在关闭服务...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP 网关关闭出错: %v", err)
	}
	done := make(chan struct{})
	go func() {
		grpcSrv.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-shutdownCtx.Done():
		grpcSrv.Stop()
	}
	log.Println("服务已停止")
}

// newHTTPHandler 把 HTTP/JSON 请求转成 gRPC 调用，而不是直接调用业务实现。
func newHTTPHandler(client greeterpb.GreeterClient) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"message":   "gRPC 服务 + HTTP/JSON 网关",
			"grpc":      "greeter.Greeter/SayHello",
			"endpoints": []string{"GET /health", "GET /api/hello?name=you"},
		})
	})

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/hello", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		reply, err := client.SayHello(ctx, &greeterpb.HelloRequest{Name: r.URL.Query().Get("name")})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "gRPC 调用失败: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"message": reply.GetMessage()})
	})

	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("写入响应失败: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// dialTarget 把监听地址（如 :50051）转换成网关可拨号的地址。
func dialTarget(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
