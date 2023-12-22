package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/streadway/amqp"
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

var (
	rabbitMQConnection    *amqp.Connection
	rabbitMQConnectionMux sync.Mutex
	upgrader              = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func initRabbitMQConnection() error {
	rabbitMQConnectionMux.Lock()
	defer rabbitMQConnectionMux.Unlock()

	if rabbitMQConnection == nil {
		conn, err := amqp.Dial("amqp://admin:witerth@witerth:5672/")
		if err != nil {
			return err
		}
		rabbitMQConnection = conn
	}

	return nil
}

func closeRabbitMQConnection() {
	rabbitMQConnectionMux.Lock()
	defer rabbitMQConnectionMux.Unlock()

	if rabbitMQConnection != nil {
		rabbitMQConnection.Close()
		rabbitMQConnection = nil
	}
}

func publishMessageToRabbitMQ(message []byte) error {
	rabbitMQConnectionMux.Lock()
	defer rabbitMQConnectionMux.Unlock()

	if rabbitMQConnection == nil {
		return fmt.Errorf("RabbitMQ connection is not initialized")
	}

	ch, err := rabbitMQConnection.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	q, err := ch.QueueDeclare(
		"my_queue", // 队列名称
		false,      // 持久性
		false,      // 自动删除
		false,      // 排它性
		false,      // 不等待服务器响应
		nil,        // 额外参数
	)
	if err != nil {
		return err
	}

	err = ch.Publish(
		"",     // 交换器
		q.Name, // 队列名称
		false,  // 强制性
		false,  // 立即发布
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        message,
		},
	)
	if err != nil {
		return err
	}

	return nil
}

func startMessageListener() {
	// 初始化 RabbitMQ 连接
	if err := initRabbitMQConnection(); err != nil {
		log.Fatalf("Failed to initialize RabbitMQ connection: %v", err)
		return
	}
	defer closeRabbitMQConnection()

	// 创建 RabbitMQ 通道
	ch, err := rabbitMQConnection.Channel()
	if err != nil {
		log.Fatalf("Failed to open RabbitMQ channel: %v", err)
		return
	}
	defer ch.Close()

	// 声明一个队列
	q, err := ch.QueueDeclare(
		"my_queue", // 队列名称
		false,      // 持久性
		false,      // 自动删除
		false,      // 排它性
		false,      // 不等待服务器响应
		nil,        // 额外参数
	)
	if err != nil {
		log.Fatalf("Failed to declare RabbitMQ queue: %v", err)
		return
	}

	// 消费队列中的消息
	msgs, err := ch.Consume(
		q.Name, // 队列名称
		"",     // 消费者名称
		true,   // 自动应答
		false,  // 排他性
		false,  // 队列删除时是否删除消息
		false,  // 不等待服务器响应
		nil,    // 额外参数
	)
	if err != nil {
		log.Fatalf("Failed to consume RabbitMQ queue: %v", err)
		return
	}

	// 处理收到的消息
	for msg := range msgs {
		// 在这里处理 RabbitMQ 收到的消息，并向所有 WebSocket 连接推送消息
		message := msg.Body
		fmt.Printf("Received RabbitMQ message: %s\n", message)

		// 获取所有 WebSocket 连接，向每个连接发送消息
		sockets.Range(func(key, value interface{}) bool {
			if conn, ok := value.(*websocket.Conn); ok {
				err := conn.WriteMessage(websocket.TextMessage, message)
				if err != nil {
					log.Println(err)
				}
			}
			return true
		})
	}
}

var sockets sync.Map

func main() {
	r := gin.Default()

	// 启动 RabbitMQ 消息监听器
	go startMessageListener()

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

		// 将消息推送到 RabbitMQ
		if err := publishMessageToRabbitMQ(messageJSON); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}

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
