// Package client is the node's gRPC connection to the Coordinator. It is a thin
// wrapper around the generated NodeService client. Reconnect and backoff live in
// the daemon, this package just dials and carries calls.
package client

import (
	"context"
	"crypto/tls"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/zonnhq/node-gui/internal/config"
	"github.com/zonnhq/node-gui/internal/resources"
	"github.com/zonnhq/node-gui/internal/version"

	colonyv1 "github.com/zonnhq/node-gui/internal/nodeproto"
)

// Client holds a live connection to the Coordinator.
type Client struct {
	conn *grpc.ClientConn
	svc  colonyv1.NodeServiceClient
}

// Target turns a configured Coordinator address into a gRPC target and whether
// to use TLS. "https://nodes.example.com" means TLS, verified against the
// system's trusted roots, on port 443 unless another is given. A bare
// "host:port" is a plain connection, for a local Coordinator or a tunnel.
func Target(addr string) (string, bool) {
	addr = strings.TrimSpace(addr)
	if rest, ok := strings.CutPrefix(addr, "https://"); ok {
		rest = strings.TrimSuffix(rest, "/")
		if _, _, err := net.SplitHostPort(rest); err != nil {
			rest = net.JoinHostPort(strings.Trim(rest, "[]"), "443")
		}
		return rest, true
	}
	return strings.TrimSuffix(strings.TrimPrefix(addr, "http://"), "/"), false
}

// Host returns just the host name of a configured Coordinator address.
func Host(addr string) string {
	target, _ := Target(addr)
	if host, _, err := net.SplitHostPort(target); err == nil {
		return host
	}
	return target
}

// Dial opens a connection to the Coordinator. With creds, every call carries
// the node's credential; without them only Enroll can succeed.
func Dial(addr string, creds *config.Credentials) (*Client, error) {
	target, useTLS := Target(addr)
	transport := insecure.NewCredentials()
	if useTLS {
		transport = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	}
	opts := []grpc.DialOption{grpc.WithTransportCredentials(transport)}
	if creds != nil {
		opts = append(opts, grpc.WithPerRPCCredentials(nodeCredential{id: creds.NodeID, secret: creds.Secret}))
	}
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, svc: colonyv1.NewNodeServiceClient(conn)}, nil
}

// nodeCredential attaches the node id and secret to every call.
type nodeCredential struct{ id, secret string }

func (c nodeCredential) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"x-node-id": c.id, "authorization": "Bearer " + c.secret}, nil
}

// RequireTransportSecurity is false so a local Coordinator or a tunnel still
// works over a plain connection; with an https:// address the credential
// travels encrypted.
func (nodeCredential) RequireTransportSecurity() bool { return false }

// Enroll exchanges a one time code from Zonn Console for this node's
// credential.
func (c *Client) Enroll(ctx context.Context, code string, cfg config.Config, specs resources.Specs, fingerprint string) (config.Credentials, error) {
	resp, err := c.svc.Enroll(ctx, &colonyv1.EnrollRequest{
		EnrollmentCode: code,
		Name:           cfg.NodeName,
		Os:             specs.OS,
		Arch:           specs.Arch,
		Fingerprint:    fingerprint,
		AppVersion:     version.Version,
		Resources:      specsToProto(specs),
		Allocated:      allocationToProto(cfg.Allocation),
		ShareCommunity: cfg.ShareCommunity,
	})
	if err != nil {
		return config.Credentials{}, err
	}
	return config.Credentials{NodeID: resp.GetNodeId(), Secret: resp.GetNodeSecret(), CoordinatorURL: cfg.CoordinatorURL}, nil
}

func specsToProto(specs resources.Specs) *colonyv1.Resources {
	return &colonyv1.Resources{
		CpuCores:    int32(specs.CPUCores),
		RamGb:       int32(specs.RAMGB),
		GpuModel:    specs.GPUModel,
		GpuMemoryGb: int32(specs.GPUMemoryGB),
		DiskGb:      int32(specs.DiskGB),
	}
}

func allocationToProto(a config.Allocation) *colonyv1.Allocation {
	return &colonyv1.Allocation{
		CpuCores:      int32(a.CPUCores),
		RamGb:         int32(a.RAMGB),
		GpuMemoryGb:   int32(a.GPUMemory),
		BandwidthMbps: int32(a.BandwidthMbps),
	}
}

// Close tears down the connection.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// RegisterResult is the outcome of a successful registration.
type RegisterResult struct {
	NodeID            string
	HeartbeatInterval time.Duration
	Colonies          []Colony
}

// Colony is a summary returned at registration.
type Colony struct {
	ID   string
	Name string
}

// Register brings this enrolled node online with its current settings.
func (c *Client) Register(ctx context.Context, nodeID string, cfg config.Config, specs resources.Specs, fingerprint, accelerator string) (RegisterResult, error) {
	resp, err := c.svc.Register(ctx, &colonyv1.RegisterRequest{
		NodeId:         nodeID,
		Name:           cfg.NodeName,
		Os:             specs.OS,
		Arch:           specs.Arch,
		Fingerprint:    fingerprint,
		AppVersion:     version.Version,
		Resources:      specsToProto(specs),
		Allocated:      allocationToProto(cfg.Allocation),
		ShareCommunity: cfg.ShareCommunity,
		Accelerator:    accelerator,
	})
	if err != nil {
		return RegisterResult{}, err
	}

	interval := time.Duration(resp.GetHeartbeatIntervalSeconds()) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	colonies := make([]Colony, 0, len(resp.GetColonies()))
	for _, c := range resp.GetColonies() {
		colonies = append(colonies, Colony{ID: c.GetId(), Name: c.GetName()})
	}
	return RegisterResult{
		NodeID:            resp.GetNodeId(),
		HeartbeatInterval: interval,
		Colonies:          colonies,
	}, nil
}

// HeartbeatStream opens the bidirectional heartbeat stream.
func (c *Client) HeartbeatStream(ctx context.Context) (colonyv1.NodeService_HeartbeatClient, error) {
	return c.svc.Heartbeat(ctx)
}

// SendHeartbeat pushes one utilization sample on an open stream and returns the
// Coordinator's acknowledgement.
func SendHeartbeat(stream colonyv1.NodeService_HeartbeatClient, nodeID string, u resources.Utilization, workerCPUCores, workerRAMGB float64, workerActive bool) (*colonyv1.HeartbeatResponse, error) {
	err := stream.Send(&colonyv1.HeartbeatRequest{
		NodeId:    nodeID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Utilization: &colonyv1.Utilization{
			CpuUsed:      u.CPUUsedCores,
			RamUsedGb:    u.RAMUsedGB,
			GpuMemUsedGb: u.GPUMemUsedGB,
			GpuTempC:     u.GPUTempC,
		},
		WorkerCpuCores: workerCPUCores,
		WorkerRamGb:    workerRAMGB,
		WorkerActive:   workerActive,
	})
	if err != nil {
		return nil, err
	}
	return stream.Recv()
}

// ReportError pushes a node side error into the Coordinator issues feed.
func (c *Client) ReportError(ctx context.Context, nodeID, level, message string) error {
	_, err := c.svc.ReportError(ctx, &colonyv1.ReportErrorRequest{
		NodeId:    nodeID,
		Level:     level,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
	return err
}

// SetOffline tells the Coordinator this node is going offline on purpose, so the
// fleet reflects it immediately rather than waiting for the reaper.
func (c *Client) SetOffline(ctx context.Context, nodeID, reason string) error {
	_, err := c.svc.SetOffline(ctx, &colonyv1.SetOfflineRequest{
		NodeId: nodeID,
		Reason: reason,
	})
	return err
}
