package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/websocket"
)

var db *sql.DB

type Message struct {
	Message  string `json:"message"`
	Time     string `json:"time"`
	UserInfo string `json:"userInfo"`
}

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

func initDB() {
	var err error
	db, err = sql.Open("mysql", "witerth:witerth@tcp(localhost:3306)/database?charset=utf8&parseTime=True&loc=Local")
	if err != nil {
		log.Fatalf("Failed to connect to the database: %v", err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatalf("Failed to ping the database: %v", err)
	}
}

func main() {
	// 初始化数据库连接
	initDB()

	// 启动定时任务，每天清理一次
	go startCleanupJob()

	r := gin.Default()

	// 示例路由
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	// POST 路由 - 获取消息
	r.POST("/history", func(c *gin.Context) {
		var requestData struct {
			Time   string `json:"time"`
			UserId string `json:"userId"`
		}

		if err := c.ShouldBindJSON(&requestData); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		// 解析时间戳
		// timeStamp, err := time.Parse(time.RFC3339, requestData.Time)
		// if err != nil {
		// 	c.JSON(400, gin.H{"error": "Invalid timestamp format"})
		// 	return
		// }

		var rows *sql.Rows

		// 如果未提供时间参数，则默认获取最后 50 条数据
		if requestData.Time == "" {
			rows, err := db.Query("SELECT message, time, user_info FROM messages ORDER BY time DESC LIMIT 10")
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			defer rows.Close()

			var messages []SendData
			for rows.Next() {
				var msg SendData
				var userInfoStr string
				err := rows.Scan(&msg.Message, &msg.Time, &userInfoStr)
				if err != nil {
					c.JSON(500, gin.H{"error": err.Error()})
					return
				}

				// 解码 user_info 字段到 UserInfo 结构体
				err = json.Unmarshal([]byte(userInfoStr), &msg.UserInfo)
				if err != nil {
					c.JSON(500, gin.H{"error": "Failed to unmarshal UserInfo"})
					return
				}

				messages = append(messages, msg)
			}

			c.JSON(200, messages)
			return
		}

		// 获取该时间和对应 userId 前的 50 条数据
		rows, err := db.Query("SELECT message, time, user_info FROM messages WHERE time < ? AND user_id = ? LIMIT 10",
			requestData.Time, requestData.UserId)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		defer rows.Close()

		var messages []SendData
		for rows.Next() {
			var msg SendData
			var userInfoStr string
			err := rows.Scan(&msg.Message, &msg.Time, &userInfoStr)
			if err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}

			// 解码 user_info 字段到 UserInfo 结构体
			err = json.Unmarshal([]byte(userInfoStr), &msg.UserInfo)
			if err != nil {
				c.JSON(500, gin.H{"error": "Failed to unmarshal UserInfo"})
				return
			}

			messages = append(messages, msg)
		}

		c.JSON(200, messages)
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

		// 将 UserInfo 转换为字符串
		userInfoStr, err := json.Marshal(sendData.UserInfo)
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to marshal UserInfo"})
			return
		}

		// 将消息保存到数据库
		_, err = db.Exec("INSERT INTO messages (message, time, user_id, user_info) VALUES (?, ?, ?, ?)",
			sendData.Message, sendData.Time, sendData.UserInfo.UserId, string(userInfoStr))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
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

func startCleanupJob() {
	for {
		// 每隔一天执行一次清理任务
		time.Sleep(24 * time.Hour)

		// 计算 24 小时前的时间
		thresholdTime := time.Now().Add(-24 * time.Hour)

		// 执行数据库清理操作
		_, err := db.Exec("DELETE FROM messages WHERE time < ?", thresholdTime)
		if err != nil {
			log.Printf("Failed to clean up messages: %v\n", err)
		} else {
			log.Println("Messages older than 24 hours have been cleaned up.")
		}
	}
}
