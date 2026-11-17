package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
)

func (s *Server) listContainers(c *gin.Context) {
	list, err := s.Docker.ListContainers(c.Request.Context())
	if err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"containers": list})
}

func (s *Server) createContainer(c *gin.Context) {
	var spec docker.ContainerSpec
	if err := c.ShouldBindJSON(&spec); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}

	created, err := s.Docker.CreateContainer(c.Request.Context(), spec)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusCreated, created)
}

// Legacy v1 body-style routes: {"container_id": "..."}.

func (s *Server) startContainerLegacy(c *gin.Context) {
	var req struct {
		ContainerID string `json:"container_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	if err := s.Docker.StartContainer(c.Request.Context(), req.ContainerID); err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"container_id": req.ContainerID, "status": "started"})
}

func (s *Server) deleteContainerLegacy(c *gin.Context) {
	var req struct {
		ContainerID string `json:"container_id" binding:"required"`
		Force       bool   `json:"force"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	if err := s.Docker.RemoveContainer(c.Request.Context(), req.ContainerID, req.Force); err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"container_id": req.ContainerID, "status": "deleted"})
}
