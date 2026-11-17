package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/pkg/stdcopy"
)

// ---- containers ----

func (s *Service) InspectContainer(ctx context.Context, id string) (container.InspectResponse, error) {
	return s.Client.ContainerInspect(ctx, id)
}

// Actions lists the lifecycle actions ContainerAction accepts.
var Actions = []string{"start", "stop", "restart", "kill", "pause", "unpause"}

func (s *Service) ContainerAction(ctx context.Context, id, action string) error {
	switch action {
	case "start":
		return s.Client.ContainerStart(ctx, id, container.StartOptions{})
	case "stop":
		return s.Client.ContainerStop(ctx, id, container.StopOptions{})
	case "restart":
		return s.Client.ContainerRestart(ctx, id, container.StopOptions{})
	case "kill":
		return s.Client.ContainerKill(ctx, id, "SIGKILL")
	case "pause":
		return s.Client.ContainerPause(ctx, id)
	case "unpause":
		return s.Client.ContainerUnpause(ctx, id)
	default:
		return fmt.Errorf("unknown action %q", action)
	}
}

// Logs returns plain (demultiplexed) log output. Close the reader when done.
func (s *Service) Logs(ctx context.Context, id string, follow bool, tail string) (io.ReadCloser, error) {
	info, err := s.Client.ContainerInspect(ctx, id)
	if err != nil {
		return nil, err
	}

	rc, err := s.Client.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       tail,
		Timestamps: true,
	})
	if err != nil {
		return nil, err
	}
	if info.Config.Tty {
		return rc, nil // TTY output is not multiplexed
	}

	pr, pw := io.Pipe()
	go func() {
		_, err := stdcopy.StdCopy(pw, pw, rc)
		rc.Close()
		pw.CloseWithError(err)
	}()
	return pr, nil
}

// Stats is a small, UI-friendly snapshot of container resource usage.
type Stats struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemUsage   uint64  `json:"mem_usage"`
	MemLimit   uint64  `json:"mem_limit"`
	MemPercent float64 `json:"mem_percent"`
	NetRx      uint64  `json:"net_rx"`
	NetTx      uint64  `json:"net_tx"`
	PIDs       uint64  `json:"pids"`
}

func (s *Service) Stats(ctx context.Context, id string) (Stats, error) {
	resp, err := s.Client.ContainerStats(ctx, id, false) // single sample with pre-CPU stats
	if err != nil {
		return Stats{}, err
	}
	defer resp.Body.Close()

	var v container.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return Stats{}, err
	}

	st := Stats{MemLimit: v.MemoryStats.Limit, PIDs: v.PidsStats.Current}

	cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage) - float64(v.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(v.CPUStats.SystemUsage) - float64(v.PreCPUStats.SystemUsage)
	cpus := float64(v.CPUStats.OnlineCPUs)
	if cpus == 0 {
		cpus = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
	}
	if cpuDelta > 0 && sysDelta > 0 {
		st.CPUPercent = cpuDelta / sysDelta * cpus * 100
	}

	// Same as `docker stats`: exclude page cache.
	st.MemUsage = v.MemoryStats.Usage
	if cache, ok := v.MemoryStats.Stats["inactive_file"]; ok && cache < st.MemUsage {
		st.MemUsage -= cache
	}
	if st.MemLimit > 0 {
		st.MemPercent = float64(st.MemUsage) / float64(st.MemLimit) * 100
	}

	for _, n := range v.Networks {
		st.NetRx += n.RxBytes
		st.NetTx += n.TxBytes
	}
	return st, nil
}

// ExecResult is the outcome of a non-interactive exec.
type ExecResult struct {
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
}

// Exec runs a command to completion and returns its combined output.
func (s *Service) Exec(ctx context.Context, id string, cmd []string) (ExecResult, error) {
	created, err := s.Client.ContainerExecCreate(ctx, id, container.ExecOptions{
		Cmd: cmd, AttachStdout: true, AttachStderr: true,
	})
	if err != nil {
		return ExecResult{}, err
	}

	hr, err := s.Client.ContainerExecAttach(ctx, created.ID, container.ExecAttachOptions{})
	if err != nil {
		return ExecResult{}, err
	}
	defer hr.Close()

	var out bytes.Buffer
	if _, err := stdcopy.StdCopy(&out, &out, hr.Reader); err != nil {
		return ExecResult{}, err
	}

	ins, err := s.Client.ContainerExecInspect(ctx, created.ID)
	if err != nil {
		return ExecResult{}, err
	}
	return ExecResult{ExitCode: ins.ExitCode, Output: out.String()}, nil
}

// ExecTTY starts an interactive shell. The caller pumps the hijacked connection.
func (s *Service) ExecTTY(ctx context.Context, id string, cmd []string) (string, types.HijackedResponse, error) {
	created, err := s.Client.ContainerExecCreate(ctx, id, container.ExecOptions{
		Cmd: cmd, Tty: true, AttachStdin: true, AttachStdout: true, AttachStderr: true,
	})
	if err != nil {
		return "", types.HijackedResponse{}, err
	}
	hr, err := s.Client.ContainerExecAttach(ctx, created.ID, container.ExecAttachOptions{Tty: true})
	return created.ID, hr, err
}

func (s *Service) ResizeExec(ctx context.Context, execID string, cols, rows uint) error {
	return s.Client.ContainerExecResize(ctx, execID, container.ResizeOptions{Width: cols, Height: rows})
}

// ---- images ----

func (s *Service) ListImages(ctx context.Context) ([]image.Summary, error) {
	return s.Client.ImageList(ctx, image.ListOptions{All: false})
}

func (s *Service) RemoveImage(ctx context.Context, id string, force bool) ([]image.DeleteResponse, error) {
	return s.Client.ImageRemove(ctx, id, image.RemoveOptions{Force: force, PruneChildren: true})
}

func (s *Service) PruneImages(ctx context.Context) (image.PruneReport, error) {
	return s.Client.ImagesPrune(ctx, filters.NewArgs(filters.Arg("dangling", "true")))
}

// ---- networks ----

func (s *Service) RemoveNetwork(ctx context.Context, id string) error {
	return s.Client.NetworkRemove(ctx, id)
}

func (s *Service) InspectNetwork(ctx context.Context, id string) (network.Inspect, error) {
	return s.Client.NetworkInspect(ctx, id, network.InspectOptions{})
}

// ---- volumes ----

type VolumeSpec struct {
	Name   string            `json:"name" binding:"required"`
	Driver string            `json:"driver"`
	Labels map[string]string `json:"labels"`
}

func (s *Service) ListVolumes(ctx context.Context) ([]*volume.Volume, error) {
	resp, err := s.Client.VolumeList(ctx, volume.ListOptions{})
	return resp.Volumes, err
}

func (s *Service) CreateVolume(ctx context.Context, v VolumeSpec) (volume.Volume, error) {
	return s.Client.VolumeCreate(ctx, volume.CreateOptions{Name: v.Name, Driver: v.Driver, Labels: v.Labels})
}

func (s *Service) RemoveVolume(ctx context.Context, name string, force bool) error {
	return s.Client.VolumeRemove(ctx, name, force)
}

// ---- system ----

func (s *Service) Info(ctx context.Context) (system.Info, error) {
	return s.Client.Info(ctx)
}
