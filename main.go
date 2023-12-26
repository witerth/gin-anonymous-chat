package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type UserInfo struct {
	UserId   string `json:"userId"`
	UserName string `json:"userName"`
	Avatar   string `json:"avatar"`
}

type SendData struct {
	Message  string   `json:"message"`
	Time     string   `json:"time"`
	UserInfo UserInfo `json:"userInfo"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var sockets sync.Map

func main() {
	r := gin.Default()

	// 示例路由
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	// POST 路由
	r.POST("/send", func(c *gin.Context) {
		var sendData SendData
		if err := c.ShouldBindJSON(&sendData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// 将 Message 对象转换为 JSON 字符串
		messageJSON, err := json.Marshal(sendData)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed"})
			return
		}

		// 将消息通过ws推到前端
		sockets.Range(func(key, value interface{}) bool {
			if conn, ok := value.(*websocket.Conn); ok {
				err := conn.WriteMessage(websocket.TextMessage, messageJSON)
				if err != nil {
					log.Println(err)
				}
			}
			return true
		})

		c.JSON(200, gin.H{"status": "success"})
	})

	// WebSocket 路由
	r.GET("/ws", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Println(err)
			return
		}
		// defer conn.Close()

		// 将 WebSocket 连接添加到全局 map
		sockets.Store(conn, conn)
		// defer sockets.Delete(conn)
	})

	// 启动 Gin 服务
	r.Run(":6565")
}
