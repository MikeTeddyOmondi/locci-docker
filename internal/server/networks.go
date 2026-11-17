package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
)

func (s *Server) listNetworks(c *gin.Context) {
	list, err := s.Docker.ListNetworks(c.Request.Context())
	if err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"networks": list})
}

func (s *Server) createNetwork(c *gin.Context) {
	var spec docker.NetworkSpec
	if err := c.ShouldBindJSON(&spec); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}

	resp, err := s.Docker.CreateNetwork(c.Request.Context(), spec)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"network_id": resp.ID, "name": spec.Name, "warnings": resp.Warning})
}
