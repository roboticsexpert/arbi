package main

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	_ "arbi/internal/metrics" // initialize metrics
)

func main() {

	// Setup Gin router
	router := gin.Default()

	router.GET("/up", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "up",
		})
	})

	// Prometheus metrics endpoint
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	router.Run() // listens on 0.0.0.0:8080 by default
}
