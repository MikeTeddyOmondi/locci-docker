package server

import (
	"bufio"
	"io"
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

func (s *Server) inspectContainer(c *gin.Context) {
	info, err := s.Docker.InspectContainer(c.Request.Context(), c.Param("id"))
	if err != nil {
		s.fail(c, statusFor(err), err)
		return
	}
	c.JSON(http.StatusOK, info)
}

// containerAction returns a handler for POST /containers/:id/<action>.
func (s *Server) containerAction(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := s.Docker.ContainerAction(c.Request.Context(), id, action); err != nil {
			s.fail(c, statusFor(err), err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"container_id": id, "action": action, "status": "ok"})
	}
}

func (s *Server) removeContainer(c *gin.Context) {
	id := c.Param("id")
	if err := s.Docker.RemoveContainer(c.Request.Context(), id, c.Query("force") == "true"); err != nil {
		s.fail(c, statusFor(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"container_id": id, "status": "deleted"})
}

// containerLogs streams plain text. ?follow=true keeps the connection open.
func (s *Server) containerLogs(c *gin.Context) {
	rc, err := s.Docker.Logs(c.Request.Context(), c.Param("id"),
		c.Query("follow") == "true", c.DefaultQuery("tail", "200"))
	if err != nil {
		s.fail(c, statusFor(err), err)
		return
	}
	defer rc.Close()

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)

	r := bufio.NewReader(rc)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			if _, werr := c.Writer.Write(line); werr != nil {
				return
			}
			c.Writer.Flush()
		}
		if err != nil {
			if err != io.EOF {
				s.Log.Debug("log stream ended", "err", err)
			}
			return
		}
	}
}

func (s *Server) containerStats(c *gin.Context) {
	st, err := s.Docker.Stats(c.Request.Context(), c.Param("id"))
	if err != nil {
		s.fail(c, statusFor(err), err)
		return
	}
	c.JSON(http.StatusOK, st)
}

func (s *Server) containerExec(c *gin.Context) {
	var req struct {
		Cmd []string `json:"cmd" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		s.fail(c, http.StatusBadRequest, err)
		return
	}
	res, err := s.Docker.Exec(c.Request.Context(), c.Param("id"), req.Cmd)
	if err != nil {
		s.fail(c, statusFor(err), err)
		return
	}
	c.JSON(http.StatusOK, res)
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
		s.fail(c, statusFor(err), err)
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
		s.fail(c, statusFor(err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"container_id": req.ContainerID, "status": "deleted"})
}
