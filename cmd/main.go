package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/streadway/amqp"
)

// Configuration
type Config struct {
	DockerHost       string
	AuthToken        string
	RabbitMQURL      string
	QueueName        string
	ServerPort       string
	TraefikDomain    string
	DeployJobTimeout int
}

// Request structures
type CreateNetworkRequest struct {
	Name   string            `json:"name" binding:"required"`
	Driver string            `json:"driver"`
	Labels map[string]string `json:"labels"`
}

type CreateContainerRequest struct {
	Name        string            `json:"name" binding:"required"`
	Image       string            `json:"image" binding:"required"`
	Ports       map[string]string `json:"ports"`
	Environment []string          `json:"environment"`
	Networks    []string          `json:"networks"`
	Subdomain   string            `json:"subdomain" binding:"required"`
	Labels      map[string]string `json:"labels"`
	Volumes     []string          `json:"volumes"`
	Command     []string          `json:"command"`
}

type StartContainerRequest struct {
	ContainerID string `json:"container_id" binding:"required"`
}

type DeleteContainerRequest struct {
	ContainerID string `json:"container_id" binding:"required"`
	Force       bool   `json:"force"`
}

// Job structure for RabbitMQ
type DeployJob struct {
	Target      string                 `json:"target"`
	Namespace   string                 `json:"namespace"`
	Replicas    int                    `json:"replicas"`
	Port        int                    `json:"port"`
	Environment string                 `json:"environment"`
	ID          string                 `json:"id"`
	Action      string                 `json:"action"`
	Container   CreateContainerRequest `json:"container,omitempty"`
	Network     CreateNetworkRequest   `json:"network,omitempty"`
	ProjectID   string                 `json:"projectId"`
	TriggeredBy string                 `json:"triggeredBy"`
	TriggeredAt string                 `json:"triggeredAt"`
}

func (dj DeployJob) validate() error {
	if dj.ID == "" {
		return errors.New("job ID is empty")
	}

	if dj.Action == "" {
		return errors.New("job Action is empty")
	}

	return nil
}

// Raw RabbitMQ message
type RabbitMQMessage struct {
	ProjectID    string    `json:"projectId"`
	DeployConfig DeployJob `json:"deployConfig"`
	Timestamp    string    `json:"timestamp"`
}

// Service structure
type DockerService struct {
	client *client.Client
	config *Config
	log    *Logger
}

const (
	maxRetries  int    = 3
	retryHeader string = "x-retry-count"
)

type Logger struct {
	handler slog.Handler
}

const (
	LevelDebug slog.Level = slog.LevelDebug
	LevelInfo  slog.Level = slog.LevelInfo
	LevelWarn  slog.Level = slog.LevelWarn
	LevelError slog.Level = slog.LevelError
)

func NewLogger(w io.Writer, minLevel slog.Level, logAttrs map[string]string) *Logger {
	handler := slog.Handler(slog.NewJSONHandler(w, &slog.HandlerOptions{AddSource: true, Level: slog.Level(minLevel)}))

	attrs := make([]slog.Attr, 0, len(logAttrs))

	for k, v := range logAttrs {
		attrs = append(attrs,
			slog.Attr{
				Key:   k,
				Value: slog.StringValue(v),
			},
		)
	}

	handler = handler.WithAttrs(attrs)

	return &Logger{
		handler: handler,
	}
}

func (log *Logger) write(ctx context.Context, level slog.Level, caller int, msg string, args ...any) {
	r := slog.NewRecord(time.Now(), level, msg, 0)

	r.Add(args...)

	err := log.handler.Handle(ctx, r)
	if err != nil {
		log.Error(ctx, err.Error())
	}
}

func (log *Logger) Debug(ctx context.Context, msg string, args ...any) {
	log.write(ctx, LevelDebug, 3, msg, args...)
}

func (log *Logger) Info(ctx context.Context, msg string, args ...any) {
	log.write(ctx, LevelInfo, 3, msg, args...)
}

func (log *Logger) Warn(ctx context.Context, msg string, args ...any) {
	log.write(ctx, LevelWarn, 3, msg, args...)
}

func (log *Logger) Error(ctx context.Context, msg string, args ...any) {
	log.write(ctx, LevelError, 3, msg, args...)
}

// Initialize Docker client
func NewDockerService(config *Config) (*DockerService, error) {
	var cli *client.Client
	var err error

	if config.DockerHost != "" {
		cli, err = client.NewClientWithOpts(
			client.WithHost(config.DockerHost),
			client.WithAPIVersionNegotiation(),
		)
	} else {
		cli, err = client.NewClientWithOpts(client.FromEnv)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	logger := NewLogger(os.Stdout, slog.LevelDebug, map[string]string{})

	return &DockerService{
		client: cli,
		config: config,
		log:    logger,
	}, nil
}

// Auth middleware
func AuthMiddleware(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization format"})
			c.Abort()
			return
		}

		if tokenParts[1] != token {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// Generate Traefik labels
func (ds *DockerService) generateTraefikLabels(subdomain string, port string) map[string]string {
	domain := fmt.Sprintf("%s.%s", subdomain, ds.config.TraefikDomain)
	serviceName := subdomain

	labels := map[string]string{
		"traefik.enable": "true",
		fmt.Sprintf("traefik.http.routers.%s.rule", subdomain):                        fmt.Sprintf("Host(`%s`)", domain),
		fmt.Sprintf("traefik.http.routers.%s.tls", subdomain):                         "true",
		fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", subdomain):            "letsencrypt",
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", serviceName): port,
	}

	return labels
}

// Create network
func (ds *DockerService) createNetwork(c *gin.Context) {
	// Use the request context so that operations are automatically cancelled if the client's connection closes
	// This helps to avoid wasting resources on requests whose response the client will never need
	ctx := c.Request.Context()

	var req CreateNetworkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"invalid request body": err.Error()})
		return
	}

	networkConfig := network.CreateOptions{
		Driver: req.Driver,
		Labels: req.Labels,
	}

	if networkConfig.Driver == "" {
		networkConfig.Driver = "bridge"
	}

	resp, err := ds.client.NetworkCreate(ctx, req.Name, networkConfig)
	if err != nil {
		ds.log.Error(ctx, "failed to create docker network", slog.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})

		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"network_id": resp.ID,
		"name":       req.Name,
		"warnings":   resp.Warning,
	})
}

// Create container
func (ds *DockerService) createContainer(c *gin.Context) {
	ctx := c.Request.Context()

	var req CreateContainerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"invalid request body": err.Error()})
		return
	}

	// Generate Traefik labels
	var mainPort string
	for _, port := range req.Ports {
		mainPort = strings.Split(port, ":")[0]
		break
	}
	if mainPort == "" {
		mainPort = "80" // default port
	}

	traefikLabels := ds.generateTraefikLabels(req.Subdomain, mainPort)

	// Merge custom labels with Traefik labels
	allLabels := make(map[string]string)
	maps.Copy(allLabels, traefikLabels)
	maps.Copy(allLabels, req.Labels)

	// Convert port mappings
	exposedPorts := nat.PortSet{}
	portBindings := nat.PortMap{}

	for containerPortStr, hostPortStr := range req.Ports {
		// Create nat.Port for the container port (assuming TCP)
		containerPort, err := nat.NewPort("tcp", containerPortStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Invalid container port %s: %v", containerPortStr, err)})
			return
		}

		// Add to exposed ports
		exposedPorts[containerPort] = struct{}{}

		// Create port binding
		portBinding := nat.PortBinding{
			HostIP:   "", // Empty means bind to all interfaces (0.0.0.0)
			HostPort: hostPortStr,
		}

		// Add to port bindings
		portBindings[containerPort] = []nat.PortBinding{portBinding}
	}

	// Container configuration
	containerConfig := &container.Config{
		Image:        req.Image,
		Env:          req.Environment,
		ExposedPorts: exposedPorts,
		Labels:       allLabels,
		Cmd:          req.Command,
	}

	// Host configuration
	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		Binds:        req.Volumes,
	}

	// Network configuration
	networkConfig := &network.NetworkingConfig{}
	if len(req.Networks) > 0 {
		endpointsConfig := make(map[string]*network.EndpointSettings)
		for _, networkName := range req.Networks {
			endpointsConfig[networkName] = &network.EndpointSettings{}
		}
		networkConfig.EndpointsConfig = endpointsConfig
	}

	// TODO: pull the Docker image
	ds.client.ImagePull(ctx, req.Image, image.PullOptions{})

	resp, err := ds.client.ContainerCreate(
		ctx,
		containerConfig,
		hostConfig,
		networkConfig,
		nil,
		req.Name,
	)

	if err != nil {
		ds.log.Error(ctx, "failed to create container", slog.String("error", err.Error()))

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"container_id": resp.ID,
		"name":         req.Name,
		"warnings":     resp.Warnings,
		"domain":       fmt.Sprintf("%s.%s", req.Subdomain, ds.config.TraefikDomain),
	})
}

// Start container
func (ds *DockerService) startContainer(c *gin.Context) {
	ctx := c.Request.Context()

	var req StartContainerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"invalid request body": err.Error()})
		return
	}

	err := ds.client.ContainerStart(ctx, req.ContainerID, container.StartOptions{})
	if err != nil {
		ds.log.Error(ctx, "failed to start container", slog.String("containerID", req.ContainerID), slog.String("error", err.Error()))

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"container_id": req.ContainerID,
		"status":       "started",
	})
}

// Delete container
func (ds *DockerService) deleteContainer(c *gin.Context) {
	ctx := c.Request.Context()

	var req DeleteContainerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Stop container first if it's running
	if req.Force {
		err := ds.client.ContainerStop(ctx, req.ContainerID, container.StopOptions{})
		if err != nil {
			ds.log.Error(
				ctx,
				"failed to forcefully stop container",
				slog.String("containerID", req.ContainerID),
				slog.String("error", err.Error()),
			)

			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	err := ds.client.ContainerRemove(ctx, req.ContainerID, container.RemoveOptions{
		Force: req.Force,
	})

	if err != nil {
		ds.log.Error(
			ctx,
			"failed to delete container",
			slog.String("containerID", req.ContainerID),
			slog.String("error", err.Error()),
		)

		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"container_id": req.ContainerID,
		"status":       "deleted",
	})
}

// List containers
func (ds *DockerService) listContainers(c *gin.Context) {
	ctx := c.Request.Context()
	containers, err := ds.client.ContainerList(ctx, container.ListOptions{
		All: true,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"containers": containers,
	})
}

// List networks
func (ds *DockerService) listNetworks(c *gin.Context) {
	ctx := c.Request.Context()

	networks, err := ds.client.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"networks": networks,
	})
}

// RabbitMQ Job processor
func (ds *DockerService) processDeployJobs(ctx context.Context, errChan chan<- error) {
	conn, err := amqp.Dial(ds.config.RabbitMQURL)
	if err != nil {
		ds.log.Error(ctx, "failed to connect to RabbitMQ", slog.String("error", err.Error()))
		errChan <- err

		return
	}

	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	ch, err := conn.Channel()
	if err != nil {
		ds.log.Error(ctx, "failed to open RabbitMQ channel", slog.String("error", err.Error()))
		errChan <- err

		return
	}

	defer ch.Close()

	queue, err := ch.QueueDeclare(
		ds.config.QueueName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		ds.log.Error(ctx, "failed to declare queue", slog.String("error", err.Error()))
		errChan <- err

		return
	}

	messages, err := ch.Consume(
		queue.Name,
		"",    // consumer
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		ds.log.Error(ctx, "failed to register consumer", slog.String("error", err.Error()))
		errChan <- err

		return
	}

	ds.log.Info(ctx, "started processing deploy jobs from RabbitMQ", slog.String("queue", queue.Name))

	for msg := range messages {
		// Extract current retry count from message headers
		retryCount := 0
		if count, ok := msg.Headers[retryHeader]; ok {
			if c, ok := count.(int32); ok {
				retryCount = int(c)
			}
		}

		var rabbitMsg RabbitMQMessage
		if err := json.Unmarshal(msg.Body, &rabbitMsg); err != nil {
			ds.log.Error(ctx, "failed to unmarshal RabbitMQ message", slog.String("error", err.Error()))
			msg.Nack(false, false)

			continue
		}

		job := rabbitMsg.DeployConfig

		err := job.validate()
		if err != nil {
			ds.log.Error(ctx, "an error occured while validating job", slog.String("error", err.Error()))
			msg.Nack(false, false)

			continue
		}

		ds.log.Info(
			ctx,
			"processing job",
			slog.String("jobID", job.ID),
			slog.String("actioan", job.Action),
			slog.Int("retryCount", retryCount),
			slog.Int("maxRetries", maxRetries),
		)

		success := ds.processJob(ctx, job)
		if success {
			msg.Ack(false)
			ds.log.Info(ctx, "job completed successfully", slog.String("jobID", job.ID))
		} else {
			ds.retryFailedJob(ctx, retryCount, msg, ch, job.ID, queue.Name)
		}
	}
}

func (ds *DockerService) retryFailedJob(
	ctx context.Context,
	retryCount int,
	msg amqp.Delivery,
	ch *amqp.Channel,
	jobID string,
	queueName string,
) {
	if retryCount > maxRetries {
		msg.Ack(false)
		ds.log.Warn(
			ctx,
			"job failed after exceeding maximum retries",
			slog.String("jobID", jobID),
			slog.String("retryCount", retryHeader),
			slog.Int("maxRetries", maxRetries),
		)
	}

	// Prepare new headers with incremented retry count
	newHeaders := make(amqp.Table, len(msg.Headers))
	if msg.Headers != nil {
		for k, v := range msg.Headers {
			newHeaders[k] = v
		}
	}
	newHeaders[retryHeader] = retryCount + 1

	err := ch.Publish(
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         msg.Body, // original body
			Headers:      newHeaders,
		},
	)
	if err != nil {
		ds.log.Error(ctx, "failed to requeue job", slog.String("jobID", jobID), slog.String("error", err.Error()))
		msg.Nack(false, false)

		return
	}

	msg.Ack(false) // Ack original message
	ds.log.Info(
		ctx,
		"job requeued successfully for retry",
		slog.String("jobID", jobID),
		slog.Int("retryCount", retryCount),
		slog.Int("maxRetries", maxRetries),
	)
}

// Process individual job
func (ds *DockerService) processJob(ctx context.Context, job DeployJob) bool {
	ctx, cancel := context.WithTimeout(ctx, time.Second*time.Duration(ds.config.DeployJobTimeout))
	defer cancel()

	log.Printf("Processing job %s with action %s (Image %s)", job.ID, job.Action, job.Container.Image)

	switch job.Action {
	case "create_network":
		networkConfig := network.CreateOptions{
			Driver: job.Network.Driver,
			Labels: job.Network.Labels,
		}
		if networkConfig.Driver == "" {
			networkConfig.Driver = "bridge"
		}

		_, err := ds.client.NetworkCreate(ctx, job.Network.Name, networkConfig)
		return err == nil

	case "create":
		// Generate Traefik labels
		var mainPort string
		for _, port := range job.Container.Ports {
			mainPort = strings.Split(port, ":")[0]
			break
		}
		if mainPort == "" {
			mainPort = "80"
		}

		traefikLabels := ds.generateTraefikLabels(job.Container.Subdomain, mainPort)
		allLabels := make(map[string]string)
		maps.Copy(allLabels, traefikLabels)
		maps.Copy(allLabels, job.Container.Labels)

		// Convert port mappings
		// exposedPorts := make(map[string]struct{})
		// portBindings := make(map[string][]string)
		exposedPorts := nat.PortSet{}
		portBindings := nat.PortMap{}

		// for containerPort, hostPort := range job.Container.Ports {
		// 	exposedPorts[containerPort+"/tcp"] = struct{}{}
		// 	portBindings[containerPort+"/tcp"] = []string{hostPort}
		// }

		containerConfig := &container.Config{
			Image:        job.Container.Image,
			Env:          job.Container.Environment,
			ExposedPorts: exposedPorts,
			Labels:       allLabels,
			Cmd:          job.Container.Command,
		}

		hostConfig := &container.HostConfig{
			PortBindings: portBindings,
			Binds:        job.Container.Volumes,
		}

		networkConfig := &network.NetworkingConfig{}
		if len(job.Container.Networks) > 0 {
			endpointsConfig := make(map[string]*network.EndpointSettings)
			for _, networkName := range job.Container.Networks {
				endpointsConfig[networkName] = &network.EndpointSettings{}
			}
			networkConfig.EndpointsConfig = endpointsConfig
		}

		_, err := ds.client.ContainerCreate(
			ctx,
			containerConfig,
			hostConfig,
			networkConfig,
			nil,
			job.Container.Name,
		)
		return err == nil

	case "start":
		err := ds.client.ContainerStart(ctx, job.Target, container.StartOptions{})
		return err == nil

	case "delete":
		ds.client.ContainerStop(ctx, job.Target, container.StopOptions{})
		err := ds.client.ContainerRemove(ctx, job.Target, container.RemoveOptions{Force: true})
		return err == nil

	default:
		ds.log.Warn(ctx, "unknown job actioan", slog.String("action", job.Action))
		return false
	}
}

// Health check endpoint
func (ds *DockerService) healthCheck(c *gin.Context) {
	_, err := ds.client.Ping(context.Background())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"docker": "connected",
	})
}

func main() {
	ctx := context.Background()
	// Load configuration from environment variables
	err := godotenv.Load()
	if err != nil {
		log.Fatalf("error loading .env file: %v", err)
	}

	config := &Config{
		DockerHost:    os.Getenv("DOCKER_HOST"),
		AuthToken:     os.Getenv("AUTH_TOKEN"),
		RabbitMQURL:   getEnvOrDefault("RABBITMQ_URL", "amqp://user:password@localhost:5672/default_vhost"),
		QueueName:     getEnvOrDefault("QUEUE_NAME", "deploy.trigger"),
		ServerPort:    getEnvOrDefault("SERVER_PORT", "8998"),
		TraefikDomain: getEnvOrDefault("TRAEFIK_DOMAIN", "locci.cloud"),
	}

	if config.AuthToken == "" {
		log.Fatal("AUTH_TOKEN environment variable is required")
	}

	// Initialize Docker service
	dockerService, err := NewDockerService(config)
	if err != nil {
		log.Fatalf("Failed to initialize Docker service: %v", err)
	}

	errChan := make(chan error)
	// Start RabbitMQ job processor in a separate goroutine
	go dockerService.processDeployJobs(ctx, errChan)

	go func() {
		if err := <-errChan; err != nil {
			log.Fatalf("deployment job encountered an unrecoverable error: %v", err)
		}
	}()

	dockerService.log.Info(
		ctx,
		"started docker wrapper service",
		slog.String("port", config.ServerPort),
		slog.String("Traefik domain", config.TraefikDomain),
		slog.String("RabbitMQ", config.QueueName),
	)

	router := setupRouter(dockerService)
	if err := router.Run(":" + config.ServerPort); err != nil {
		log.Fatalf("failed to start server: %v", err)
	}
}

func setupRouter(dockerService *DockerService) *gin.Engine {
	ginMode := getEnvOrDefault("GIN_MODE", "debug")
	// Setup Gin router
	gin.SetMode(ginMode)
	router := gin.Default()

	// Health check endpoint (no auth required)
	router.GET("/health", dockerService.healthCheck)

	// API routes with authentication
	api := router.Group("/api/v1")
	api.Use(AuthMiddleware(dockerService.config.AuthToken))
	{
		// Network operations
		api.POST("/networks", dockerService.createNetwork)
		api.GET("/networks", dockerService.listNetworks)

		// Container operations
		api.POST("/containers", dockerService.createContainer)
		api.POST("/containers/start", dockerService.startContainer)
		api.DELETE("/containers", dockerService.deleteContainer)
		api.GET("/containers", dockerService.listContainers)
	}

	return router
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
