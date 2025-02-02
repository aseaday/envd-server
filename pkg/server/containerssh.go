// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"

	"github.com/cockroachdb/errors"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v4"
	"github.com/sirupsen/logrus"
	"go.containerssh.io/libcontainerssh/auth"
	"go.containerssh.io/libcontainerssh/config"
	"golang.org/x/crypto/ssh"

	"github.com/tensorchord/envd-server/sshname"
)

// @Summary     Update the config of containerssh.
// @Description It is called by the containerssh webhook. and is not expected to be used externally.
// @Tags        ssh-internal
// @Accept      json
// @Produce     json
// @Param       request body config.Request true "query params"
// @Success     200
// @Router      /config [post]
func (s *Server) OnConfig(c *gin.Context) {
	var req config.Request
	if err := c.BindJSON(&req); err != nil {
		c.JSON(500, err)
		return
	}

	_, name, err := sshname.GetInfo(req.Username)
	if err != nil {
		c.JSON(500, err)
		return
	}

	cfg := config.AppConfig{
		Backend: "sshproxy",
		SSHProxy: config.SSHProxyConfig{
			Server:   name,
			Port:     2222,
			Username: "envd",
		},
	}
	fingerprints := s.serverFingerPrints
	cfg.SSHProxy.AllowedHostKeyFingerprints = fingerprints
	res := config.ResponseBody{
		Config: cfg,
	}
	c.JSON(200, res)
}

// @Summary     authenticate the public key.
// @Description It is called by the containerssh webhook. and is not expected to be used externally.
// @Tags        ssh-internal
// @Accept      json
// @Produce     json
// @Param       request body     auth.PublicKeyAuthRequest true "query params"
// @Success     200     {object} auth.ResponseBody
// @Router      /pubkey [post]
func (s *Server) OnPubKey(c *gin.Context) {
	var req auth.PublicKeyAuthRequest
	if err := c.BindJSON(&req); err != nil {
		logrus.WithError(err).WithField("req", req).Error("failed to bind the json")
		c.JSON(500, err)
		return
	}

	owner, _, err := sshname.GetInfo(req.Username)
	if err != nil {
		c.JSON(500, err)
		return
	}

	user, err := s.Queries.GetUser(context.Background(), owner)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logrus.WithError(err).Error("user not found")
			c.JSON(500, "user not found")
			return
		} else {
			logrus.WithError(err).Errorf("db query failed: %v", err)
			c.JSON(500, "Internal error")
			return
		}
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(req.PublicKey.PublicKey))
	if err != nil {
		logrus.WithError(err).Error("failed to parse key")
		c.JSON(500, err)
		return
	}
	if subtle.ConstantTimeCompare(key.Marshal(), user.PublicKey) == 1 {
		res := auth.ResponseBody{
			Success: true,
		}
		c.JSON(200, res)
		return
	}
	res := auth.ResponseBody{
		Success: false,
	}
	c.JSON(200, res)
}

func PrettyStruct(data interface{}) (string, error) {
	val, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return "", err
	}
	return string(val), nil
}
