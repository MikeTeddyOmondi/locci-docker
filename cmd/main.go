package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	// "time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/streadway/amqp"
)

// Configuration
type Config struct {
	DockerHost    string
	AuthToken     string
	RabbitMQURL   string
	QueueName     string
	ServerPort    string
	TraefikDomain string
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
	ID        string                 `json:"id"`
	Action    string                 `json:"action"` // "create", "start", "delete", "create_network"
	Container CreateContainerRequest `json:"container,omitempty"`
	Network   CreateNetworkRequest   `json:"network,omitempty"`
	Target    string                 `json:"target,omitempty"` // container_id for start/delete operations
	// RetryCount int                    `json:"retry_count"` // The total count of retries for the job when it fails
}

// Service structure
type DockerService struct {
	client *client.Client
	config *Config
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

	return &DockerService{
		client: cli,
		config: config,
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
	// serviceName := fmt.Sprintf("%s-service", subdomain)
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
	var req CreateNetworkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	networkConfig := network.CreateOptions{
		Driver: req.Driver,
		Labels: req.Labels,
	}

	if networkConfig.Driver == "" {
		networkConfig.Driver = "bridge"
	}

	resp, err := ds.client.NetworkCreate(context.Background(), req.Name, networkConfig)
	if err != nil {
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
	var req CreateContainerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
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
	for k, v := range traefikLabels {
		allLabels[k] = v
	}
	for k, v := range req.Labels {
		allLabels[k] = v
	}

	// Convert port mappings
	// exposedPorts := make(map[string]struct{})
	// portBindings := make(map[string][]string)
	exposedPorts := nat.PortSet{}
	portBindings := nat.PortMap{}

	// for containerPort, hostPort := range req.Ports {
	// 	exposedPorts[containerPort+"/tcp"] = struct{}{}
	// 	portBindings[containerPort+"/tcp"] = []string{hostPort}
	// }
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

	resp, err := ds.client.ContainerCreate(
		context.Background(),
		containerConfig,
		hostConfig,
		networkConfig,
		nil,
		req.Name,
	)

	if err != nil {
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
	var req StartContainerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := ds.client.ContainerStart(context.Background(), req.ContainerID, container.StartOptions{})
	if err != nil {
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
	var req DeleteContainerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Stop container first if it's running
	if req.Force {
		ds.client.ContainerStop(context.Background(), req.ContainerID, container.StopOptions{})
	}

	err := ds.client.ContainerRemove(context.Background(), req.ContainerID, container.RemoveOptions{
		Force: req.Force,
	})

	if err != nil {
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
	containers, err := ds.client.ContainerList(context.Background(), container.ListOptions{
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
	networks, err := ds.client.NetworkList(context.Background(), network.ListOptions{})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"networks": networks,
	})
}

// RabbitMQ job processor
func (ds *DockerService) processDeployJobs() {
	conn, err := amqp.Dial(ds.config.RabbitMQURL)
	if err != nil {
		log.Printf("Failed to connect to RabbitMQ: %v", err)
		return
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Printf("Failed to open RabbitMQ channel: %v", err)
		return
	}
	defer ch.Close()

	// Declare queue
	q, err := ch.QueueDeclare(
		ds.config.QueueName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		log.Printf("Failed to declare queue: %v", err)
		return
	}

	msgs, err := ch.Consume(
		q.Name,
		"",    // consumer
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		log.Printf("Failed to register consumer: %v", err)
		return
	}

	const maxRetries = 3
	const retryHeader = "x-retry-count"

	log.Println("Started processing deploy jobs from RabbitMQ")

	for msg := range msgs {
		// Extract current retry count from message headers
		retryCount := 0
		if count, ok := msg.Headers[retryHeader]; ok {
			if c, ok := count.(int32); ok {
				retryCount = int(c)
			}
		}

		var job DeployJob
		if err := json.Unmarshal(msg.Body, &job); err != nil {
			log.Printf("Failed to unmarshal job: %v", err)
			msg.Nack(false, false)
			continue
		}

		log.Printf("Processing job %s with action %s (retry %d/%d)", job.ID, job.Action, retryCount, maxRetries)

		success := ds.processJob(job)
		if success {
			msg.Ack(false)
			log.Printf("Job %s completed successfully", job.ID)
		} else {
			// Handle retry logic
			if retryCount < maxRetries {
				// Prepare new headers with incremented retry count
				newHeaders := make(amqp.Table)
				if msg.Headers != nil {
					// Copy existing headers
					for k, v := range msg.Headers {
						newHeaders[k] = v
					}
				}
				newHeaders[retryHeader] = retryCount + 1

				// Requeue with updated headers
				err = ch.Publish(
					"",     // exchange
					q.Name, // routing key
					false,  // mandatory
					false,  // immediate
					amqp.Publishing{
						DeliveryMode: amqp.Persistent,
						ContentType:  "application/json",
						Body:         msg.Body, // original body
						Headers:      newHeaders,
					},
				)
				if err != nil {
					log.Printf("Failed to requeue job: %v", err)
					msg.Nack(false, false)
				} else {
					msg.Ack(false) // Ack original message
					log.Printf("Job %s requeued for retry (%d/%d)", job.ID, retryCount+1, maxRetries)
				}
			} else {
				// Max retries exceeded
				msg.Ack(false)
				log.Printf("Job %s failed after %d retries - giving up", job.ID, maxRetries)
			}
		}
	}
}

// Process individual job
func (ds *DockerService) processJob(job DeployJob) bool {
	ctx := context.Background()

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
		for k, v := range traefikLabels {
			allLabels[k] = v
		}
		for k, v := range job.Container.Labels {
			allLabels[k] = v
		}

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
		log.Printf("Unknown job action: %s", job.Action)
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
	// Load configuration from environment variables
	err := godotenv.Load()
	if err != nil {
		log.Println("Error loading .env file")
	}

	config := &Config{
		DockerHost:    os.Getenv("DOCKER_HOST"),
		AuthToken:     os.Getenv("AUTH_TOKEN"),
		RabbitMQURL:   getEnvOrDefault("RABBITMQ_URL", "amqp://user:password@localhost:5672/default_vhost"),
		QueueName:     getEnvOrDefault("QUEUE_NAME", "deploy_queue"),
		ServerPort:    getEnvOrDefault("SERVER_PORT", "8998"),
		TraefikDomain: getEnvOrDefault("TRAEFIK_DOMAIN", "locci-cloud.localhost"),
	}

	if config.AuthToken == "" {
		log.Fatal("AUTH_TOKEN environment variable is required")
	}

	// Initialize Docker service
	dockerService, err := NewDockerService(config)
	if err != nil {
		log.Fatalf("Failed to initialize Docker service: %v", err)
	}

	// Start RabbitMQ job processor in a separate goroutine
	go dockerService.processDeployJobs()

	// Setup Gin router
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	// Health check endpoint (no auth required)
	router.GET("/health", dockerService.healthCheck)

	// API routes with authentication
	api := router.Group("/api/v1")
	api.Use(AuthMiddleware(config.AuthToken))
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

	log.Printf("Starting Docker wrapper service on port %s", config.ServerPort)
	log.Printf("Traefik domain: %s", config.TraefikDomain)
	log.Printf("RabbitMQ queue: %s", config.QueueName)

	if err := router.Run(":" + config.ServerPort); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
