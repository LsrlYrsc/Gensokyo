package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/hoshinonyaruko/gensokyo/Processor"
	"github.com/hoshinonyaruko/gensokyo/callapi"
	"github.com/hoshinonyaruko/gensokyo/config"
	"github.com/hoshinonyaruko/gensokyo/wsclient"
	"github.com/tencent-connect/botgo/openapi"
)

type WebSocketServerClient struct {
	Conn  *websocket.Conn
	API   openapi.OpenAPI
	APIv2 openapi.OpenAPI
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// 确保WebSocketServerClient实现了interfaces.WebSocketServerClienter接口
var _ callapi.WebSocketServerClienter = &WebSocketServerClient{}

// 使用闭包结构 因为gin需要c *gin.Context固定签名
func WsHandlerWithDependencies(api openapi.OpenAPI, apiV2 openapi.OpenAPI, p *Processor.Processors) gin.HandlerFunc {
	return func(c *gin.Context) {
		wsHandler(api, apiV2, p, c)
	}
}

func wsHandler(api openapi.OpenAPI, apiV2 openapi.OpenAPI, p *Processor.Processors, c *gin.Context) {
	// 先获取请求头中的token
	tokenFromHeader := c.Request.Header.Get("Authorization")
	if tokenFromHeader == "" || !strings.HasPrefix(tokenFromHeader, "Token ") {
		log.Printf("Connection failed due to missing or invalid token. Headers: %v, Provided token: %s", c.Request.Header, tokenFromHeader)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid token"})
		return
	}

	// 从 "Token " 后面提取真正的token值
	token := strings.TrimPrefix(tokenFromHeader, "Token ")

	// 使用GetWsServerToken()来获取有效的token
	validToken := config.GetWsServerToken()
	if token != validToken {
		log.Printf("Connection failed due to incorrect token. Headers: %v, Provided token: %s", c.Request.Header, tokenFromHeader)
		c.JSON(http.StatusForbidden, gin.H{"error": "Incorrect token"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to set websocket upgrade: %+v", err)
		return
	}

	clientIP := c.ClientIP()
	log.Printf("WebSocket client connected. IP: %s", clientIP)

	// 创建WebSocketServerClient实例
	client := &WebSocketServerClient{
		Conn:  conn,
		API:   api,
		APIv2: apiV2,
	}
	// 将此客户端添加到Processor的WsServerClients列表中
	p.WsServerClients = append(p.WsServerClients, client)

	defer conn.Close()

	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			return
		}

		if messageType == websocket.TextMessage {
			processWSMessage(client, p)
		}
	}
}

func processWSMessage(client *WebSocketServerClient, msg []byte) {
	var message callapi.ActionMessage
	err := json.Unmarshal(msg, &message)
	if err != nil {
		log.Printf("Error unmarshalling message: %v, Original message: %s", err, string(msg))
		return
	}

	fmt.Println("Received from WebSocket onebotv11 client:", wsclient.TruncateMessage(message, 500))
	// 调用callapi
	callapi.CallAPIFromDict(client, client.API, client.APIv2, message)
}

// 发信息给client
func (c *WebSocketServerClient) SendMessage(message map[string]interface{}) error {
	msgBytes, err := json.Marshal(message)
	if err != nil {
		log.Println("Error marshalling message:", err)
		return err
	}
	return c.Conn.WriteMessage(websocket.TextMessage, msgBytes)
}
