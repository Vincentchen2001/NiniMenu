package main

import (
	"fmt"
	"ninimenu/internal/config"
	"ninimenu/internal/database"
	"ninimenu/internal/routes"
	"ninimenu/internal/services"

	"github.com/gin-gonic/gin"
)

func main() {
	config.Load()

	if err := database.Init(); err != nil {
		panic(fmt.Sprintf("数据库初始化失败: %v", err))
	}
	// Settles the rule seed migration at boot instead of on first request;
	// a failure here is retried by ListMenuRules, so only warn.
	if err := services.EnsureDefaultMenuRules(); err != nil {
		fmt.Printf("默认推荐规则初始化失败（首次访问时会重试）: %v\n", err)
	}

	r := gin.Default()
	routes.Setup(r)

	fmt.Printf("NiniMenu 启动成功！访问 http://localhost:%s\n", config.C.Port)
	if err := r.Run(":" + config.C.Port); err != nil {
		panic(fmt.Sprintf("启动失败: %v", err))
	}
}
