package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/MikeTeddyOmondi/locci-docker/internal/docker"
)

// ---- images ----

func (s *Server) listImages(c *gin.Context) {
	list, err := s.Docker.ListImages(c.Request.Context())
	if err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"images": list})
}

func (s *Server) pullImage(c *gin.Context) {
	var req struct {
		Image string `json:"image" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	if err := s.Docker.Pull(c.Request.Context(), req.Image); err != nil {
		s.fail(c, http.StatusBadGateway, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"image": req.Image, "status": "pulled"})
}

func (s *Server) removeImage(c *gin.Context) {
	resp, err := s.Docker.RemoveImage(c.Request.Context(), c.Param("id"), c.Query("force") == "true")
	if err != nil {
		s.fail(c, statusFor(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": resp})
}

func (s *Server) pruneImages(c *gin.Context) {
	report, err := s.Docker.PruneImages(c.Request.Context())
	if err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, report)
}

// ---- networks ----

func (s *Server) inspectNetwork(c *gin.Context) {
	n, err := s.Docker.InspectNetwork(c.Request.Context(), c.Param("id"))
	if err != nil {
		s.fail(c, statusFor(err), err)
		return
	}
	c.JSON(http.StatusOK, n)
}

func (s *Server) removeNetwork(c *gin.Context) {
	if err := s.Docker.RemoveNetwork(c.Request.Context(), c.Param("id")); err != nil {
		s.fail(c, statusFor(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"network_id": c.Param("id"), "status": "deleted"})
}

// ---- volumes ----

func (s *Server) listVolumes(c *gin.Context) {
	list, err := s.Docker.ListVolumes(c.Request.Context())
	if err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"volumes": list})
}

func (s *Server) createVolume(c *gin.Context) {
	var spec docker.VolumeSpec
	if err := c.ShouldBindJSON(&spec); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	v, err := s.Docker.CreateVolume(c.Request.Context(), spec)
	if err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusCreated, v)
}

func (s *Server) removeVolume(c *gin.Context) {
	if err := s.Docker.RemoveVolume(c.Request.Context(), c.Param("name"), c.Query("force") == "true"); err != nil {
		s.fail(c, statusFor(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"volume": c.Param("name"), "status": "deleted"})
}

// ---- system ----

func (s *Server) systemInfo(c *gin.Context) {
	info, err := s.Docker.Info(c.Request.Context())
	if err != nil {
		s.fail(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, info)
}
