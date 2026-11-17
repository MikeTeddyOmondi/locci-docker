// Package jobs consumes deploy jobs from RabbitMQ and runs them against Docker.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/streadway/amqp"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
)

const (
	maxRetries  = 3
	retryHeader = "x-retry-count"
)

// Job is one deploy instruction.
type Job struct {
	Target      string               `json:"target"`
	Namespace   string               `json:"namespace"`
	Replicas    int                  `json:"replicas"`
	Port        int                  `json:"port"`
	Environment string               `json:"environment"`
	ID          string               `json:"id"`
	Action      string               `json:"action"`
	Container   docker.ContainerSpec `json:"container,omitempty"`
	Network     docker.NetworkSpec   `json:"network,omitempty"`
	ProjectID   string               `json:"projectId"`
	TriggeredBy string               `json:"triggeredBy"`
	TriggeredAt string               `json:"triggeredAt"`
}

// Message is the raw RabbitMQ payload.
type Message struct {
	ProjectID    string `json:"projectId"`
	DeployConfig Job    `json:"deployConfig"`
	Timestamp    string `json:"timestamp"`
}

type Consumer struct {
	URL     string
	Queue   string
	Timeout time.Duration
	Docker  *docker.Service
	Log     *slog.Logger
}

// Run blocks, consuming jobs until the connection drops or ctx is done.
func (c *Consumer) Run(ctx context.Context) error {
	conn, err := amqp.Dial(c.URL)
	if err != nil {
		return fmt.Errorf("rabbitmq dial: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq channel: %w", err)
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(c.Queue, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("declare queue: %w", err)
	}

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}

	c.Log.Info("consuming deploy jobs", "queue", q.Name)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-msgs:
			if !ok {
				return errors.New("rabbitmq channel closed")
			}
			c.handle(ctx, ch, q.Name, msg)
		}
	}
}

func (c *Consumer) handle(ctx context.Context, ch *amqp.Channel, queue string, msg amqp.Delivery) {
	var m Message
	if err := json.Unmarshal(msg.Body, &m); err != nil || m.DeployConfig.ID == "" || m.DeployConfig.Action == "" {
		c.Log.Error("dropping invalid job", "err", err)
		msg.Nack(false, false)
		return
	}

	job := m.DeployConfig
	retries := 0
	if n, ok := msg.Headers[retryHeader].(int32); ok {
		retries = int(n)
	}

	c.Log.Info("processing job", "id", job.ID, "action", job.Action, "retry", retries)

	err := c.Process(ctx, job)
	if err == nil {
		msg.Ack(false)
		c.Log.Info("job done", "id", job.ID)
		return
	}

	c.Log.Error("job failed", "id", job.ID, "err", err)

	if retries >= maxRetries {
		msg.Ack(false)
		c.Log.Warn("job gave up after max retries", "id", job.ID, "retries", retries)
		return
	}

	headers := amqp.Table{}
	for k, v := range msg.Headers {
		headers[k] = v
	}
	headers[retryHeader] = int32(retries + 1)

	err = ch.Publish("", queue, false, false, amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		Body:         msg.Body,
		Headers:      headers,
	})
	if err != nil {
		c.Log.Error("requeue failed", "id", job.ID, "err", err)
		msg.Nack(false, false)
		return
	}
	msg.Ack(false)
}

// Process runs one job.
func (c *Consumer) Process(ctx context.Context, job Job) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	switch job.Action {
	case "create_network":
		_, err := c.Docker.CreateNetwork(ctx, job.Network)
		return err
	case "create":
		_, err := c.Docker.CreateContainer(ctx, job.Container)
		return err
	case "start":
		return c.Docker.StartContainer(ctx, job.Target)
	case "delete":
		return c.Docker.RemoveContainer(ctx, job.Target, true)
	default:
		return fmt.Errorf("unknown action %q", job.Action)
	}
}
