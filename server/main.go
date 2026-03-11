package main

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Game constants
const (
	MaxPlayersPerGame = 10
	MinPlayersPerGame = 3
	NumClueRounds     = 3
	ClueTimeLimit     = 30 // seconds
	VoteTimeLimit     = 45 // seconds
)

// Game states
const (
	StateLobby       = "lobby"
	StateAssignRoles = "assign_roles"
	StateClueRound   = "clue_round"
	StateVoting      = "voting"
	StateReveal      = "reveal"
)

// Secret words pool
var secretWords = []string{
	"pizza", "elephant", "beach", "mountain", "guitar", "basketball", 
	"computer", "sunset", "coffee", "bicycle", "castle", "painting",
	"airplane", "birthday", "ocean", "chocolate", "wedding", "rainforest",
	"smartphone", "telescope", "astronaut", "waterfall", "library", "fireworks",
}

// Client represents a connected player
type Client struct {
	conn     *websocket.Conn
	game     *Game
	playerID string
	name     string
	isHost   bool
	mutex    sync.Mutex
	send     chan []byte
}

// Player represents a player in the game
type Player struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	IsHost   bool   `json:"isHost"`
	IsActive bool   `json:"isActive"`
}

// Game represents a game room
type Game struct {
	ID             string              `json:"id"`
	Players        map[string]*Player  `json:"players"`
	State          string              `json:"state"`
	SecretWord     string              `json:"secretWord,omitempty"`
	ImpostorID     string              `json:"impostorId,omitempty"`
	Round          int                 `json:"round"`
	TurnIndex      int                 `json:"turnIndex"`
	Clues          map[string][]string `json:"clues"` // playerID -> clues
	Votes          map[string]string   `json:"votes"` // voterID -> votedID
	RoundStartTime time.Time           `json:"roundStartTime"`
	TimeLimit      int                 `json:"timeLimit"`
	mutex          sync.Mutex
	clients        map[string]*Client
}

// Message types
const (
	TypeJoinGame     = "join_game"
	TypeGameState    = "game_state"
	TypeStartGame    = "start_game"
	TypeSubmitClue   = "submit_clue"
	TypeSubmitVote   = "submit_vote"
	TypeNextRound    = "next_round"
	TypeRestartGame  = "restart_game"
	TypeLeaveGame    = "leave_game"
	TypeError        = "error"
	TypePlayerJoined = "player_joined"
	TypePlayerLeft   = "player_left"
)

// Message represents a WebSocket message
type Message struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

var (
	games      = make(map[string]*Game)
	gamesMutex sync.Mutex
	upgrader   = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}
)

func main() {
	rand.Seed(time.Now().UnixNano())

	http.HandleFunc("/ws", handleWebSocket)
	http.Handle("/", http.FileServer(http.Dir("./static")))

	log.Println("Server started on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}

// Generate a random 6-character game code
func generateGameCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	code := make([]byte, 6)
	for i := range code {
		code[i] = chars[rand.Intn(len(chars))]
	}
	return string(code)
}

// Create a new game
func createGame() *Game {
	gameCode := generateGameCode()
	
	gamesMutex.Lock()
	defer gamesMutex.Unlock()
	
	// Ensure uniqueness
	for _, exists := games[gameCode]; exists; {
		gameCode = generateGameCode()
	}
	
	game := &Game{
		ID:      gameCode,
		Players: make(map[string]*Player),
		State:   StateLobby,
		Round:   0,
		Clues:   make(map[string][]string),
		Votes:   make(map[string]string),
		clients: make(map[string]*Client),
	}
	
	games[gameCode] = game
	return game
}

// Get a game by ID
func getGame(gameID string) (*Game, bool) {
	gamesMutex.Lock()
	defer gamesMutex.Unlock()
	
	game, exists := games[gameID]
	return game, exists
}

// Handle WebSocket connections
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	
	clientID := generateGameCode() // Reusing the function to generate a unique ID
	client := &Client{
		conn:     conn,
		playerID: clientID,
		send:     make(chan []byte, 256),
	}
	
	go client.readPump()
	go client.writePump()
}

// Handle incoming WebSocket messages
func (c *Client) readPump() {
	defer func() {
		c.disconnect()
		c.conn.Close()
	}()
	
	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
		
		c.handleMessage(message)
	}
}

// Send messages to the WebSocket client
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)
			
			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Handle messages from the client
func (c *Client) handleMessage(data []byte) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		c.sendError("Invalid message format")
		return
	}
	
	switch msg.Type {
	case TypeJoinGame:
		c.handleJoinGame(msg.Payload)
	case TypeStartGame:
		c.handleStartGame()
	case TypeSubmitClue:
		c.handleSubmitClue(msg.Payload)
	case TypeSubmitVote:
		c.handleSubmitVote(msg.Payload)
	case TypeNextRound:
		c.handleNextRound()
	case TypeRestartGame:
		c.handleRestartGame()
	case TypeLeaveGame:
		c.disconnect()
	default:
		c.sendError("Unknown message type")
	}
}

// Handle client joining a game
func (c *Client) handleJoinGame(payload interface{}) {
	data, ok := payload.(map[string]interface{})
	if !ok {
		c.sendError("Invalid join payload")
		return
	}
	
	name, _ := data["name"].(string)
	gameID, _ := data["gameId"].(string)
	createNew, _ := data["createNew"].(bool)
	
	if name == "" {
		c.sendError("Player name is required")
		return
	}
	
	var game *Game
	var exists bool
	
	if createNew {
		game = createGame()
		c.isHost = true
	} else {
		if gameID == "" {
			c.sendError("Game ID is required to join a game")
			return
		}
		
		game, exists = getGame(gameID)
		if !exists {
			c.sendError("Game not found")
			return
		}
		
		game.mutex.Lock()
		if len(game.Players) >= MaxPlayersPerGame {
			game.mutex.Unlock()
			c.sendError("Game is full")
			return
		}
		
		if game.State != StateLobby {
			game.mutex.Unlock()
			c.sendError("Cannot join game in progress")
			return
		}
		game.mutex.Unlock()
	}
	
	c.name = name
	c.game = game
	
	// Add player to the game
	game.mutex.Lock()
	player := &Player{
		ID:       c.playerID,
		Name:     c.name,
		IsHost:   c.isHost,
		IsActive: true,
	}
	game.Players[c.playerID] = player
	game.clients[c.playerID] = c
	game.mutex.Unlock()
	
	// Notify the player they've joined successfully
	c.sendMessage(Message{
		Type: TypeJoinGame,
		Payload: map[string]interface{}{
			"playerId": c.playerID,
			"gameId":   game.ID,
			"isHost":   c.isHost,
		},
	})
	
	// Broadcast to all players in the game
	game.broadcastPlayerJoined(player)
	game.broadcastGameState()
}

// Handle game start request
func (c *Client) handleStartGame() {
	if c.game == nil {
		c.sendError("Not in a game")
		return
	}
	
	c.game.mutex.Lock()
	defer c.game.mutex.Unlock()
	
	// Only the host can start the game
	if !c.isHost {
		c.sendError("Only the host can start the game")
		return
	}
	
	// Check if we have enough players
	if len(c.game.Players) < MinPlayersPerGame {
		c.sendError("Need at least 3 players to start")
		return
	}
	
	// Start the game
	c.game.startGame()
}

// Start the game
func (g *Game) startGame() {
	// Pick a random secret word
	g.SecretWord = secretWords[rand.Intn(len(secretWords))]
	
	// Reset game state
	g.State = StateAssignRoles
	g.Round = 1
	g.TurnIndex = 0
	g.Clues = make(map[string][]string)
	g.Votes = make(map[string]string)
	
	// Select a random impostor
	var playerIDs []string
	for id := range g.Players {
		playerIDs = append(playerIDs, id)
	}
	g.ImpostorID = playerIDs[rand.Intn(len(playerIDs))]
	
	// Assign roles and send to clients
	for id, client := range g.clients {
		isImpostor := id == g.ImpostorID
		word := ""
		if !isImpostor {
			word = g.SecretWord
		}
		
		client.sendMessage(Message{
			Type: "role_assignment",
			Payload: map[string]interface{}{
				"isImpostor": isImpostor,
				"secretWord": word,
			},
		})
	}
	
	// Move to the first clue round
	g.startClueRound()
}

// Start a clue round
func (g *Game) startClueRound() {
	g.State = StateClueRound
	g.RoundStartTime = time.Now()
	g.TimeLimit = ClueTimeLimit
	
	// Get active player IDs in a slice for turn management
	var playerIDs []string
	for id, player := range g.Players {
		if player.IsActive {
			playerIDs = append(playerIDs, id)
		}
	}
	
	// If we reached end of player list, move to next round
	if g.TurnIndex >= len(playerIDs) {
		g.TurnIndex = 0
		g.Round++
		
		// If we've completed all clue rounds, move to voting
		if g.Round > NumClueRounds {
			g.startVoting()
			return
		}
	}
	
	g.broadcastGameState()
	
	// Set a timer for the turn
	time.AfterFunc(time.Duration(ClueTimeLimit)*time.Second, func() {
		g.mutex.Lock()
		defer g.mutex.Unlock()
		
		// Skip to next player if time ran out
		currentPlayerID := playerIDs[g.TurnIndex]
		if g.State == StateClueRound && len(g.Clues[currentPlayerID]) < g.Round {
			g.Clues[currentPlayerID] = append(g.Clues[currentPlayerID], "⏱️")  // Clock emoji to indicate time out
			g.TurnIndex++
			g.startClueRound()
		}
	})
}

// Start the voting phase
func (g *Game) startVoting() {
	g.State = StateVoting
	g.RoundStartTime = time.Now()
	g.TimeLimit = VoteTimeLimit
	g.broadcastGameState()
	
	// Set a timer for voting
	time.AfterFunc(time.Duration(VoteTimeLimit)*time.Second, func() {
		g.mutex.Lock()
		defer g.mutex.Unlock()
		
		if g.State == StateVoting {
			g.endVoting()
		}
	})
}

// End the voting phase and reveal results
func (g *Game) endVoting() {
	g.State = StateReveal
	g.broadcastGameState()
}

// Handle clue submission
func (c *Client) handleSubmitClue(payload interface{}) {
	if c.game == nil {
		c.sendError("Not in a game")
		return
	}
	
	data, ok := payload.(map[string]interface{})
	if !ok {
		c.sendError("Invalid clue payload")
		return
	}
	
	clue, _ := data["clue"].(string)
	if clue == "" {
		c.sendError("Clue cannot be empty")
		return
	}
	
	c.game.mutex.Lock()
	defer c.game.mutex.Unlock()
	
	// Check if it's this player's turn
	if c.game.State != StateClueRound {
		c.sendError("Not in clue round")
		return
	}
	
	// Get active player IDs in a slice
	var playerIDs []string
	for id, player := range c.game.Players {
		if player.IsActive {
			playerIDs = append(playerIDs, id)
		}
	}
	
	if c.game.TurnIndex >= len(playerIDs) || playerIDs[c.game.TurnIndex] != c.playerID {
		c.sendError("Not your turn")
		return
	}
	
	// Record the clue
	if _, exists := c.game.Clues[c.playerID]; !exists {
		c.game.Clues[c.playerID] = make([]string, 0)
	}
	c.game.Clues[c.playerID] = append(c.game.Clues[c.playerID], clue)
	
	// Move to next player
	c.game.TurnIndex++
	c.game.startClueRound()
}

// Handle vote submission
func (c *Client) handleSubmitVote(payload interface{}) {
	if c.game == nil {
		c.sendError("Not in a game")
		return
	}
	
	data, ok := payload.(map[string]interface{})
	if !ok {
		c.sendError("Invalid vote payload")
		return
	}
	
	votedForID, _ := data["votedFor"].(string)
	if votedForID == "" {
		c.sendError("Must vote for a player")
		return
	}
	
	c.game.mutex.Lock()
	defer c.game.mutex.Unlock()
	
	if c.game.State != StateVoting {
		c.sendError("Not in voting phase")
		return
	}
	
	// Record the vote
	c.game.Votes[c.playerID] = votedForID
	
	// Check if all active players have voted
	allVoted := true
	for id, player := range c.game.Players {
		if player.IsActive && c.game.Votes[id] == "" {
			allVoted = false
			break
		}
	}
	
	if allVoted {
		c.game.endVoting()
	} else {
		c.game.broadcastGameState()
	}
}

// Handle request to start next round
func (c *Client) handleNextRound() {
	if c.game == nil {
		c.sendError("Not in a game")
		return
	}
	
	c.game.mutex.Lock()
	defer c.game.mutex.Unlock()
	
	// Only the host can start a new round
	if !c.isHost {
		c.sendError("Only the host can start a new round")
		return
	}
	
	if c.game.State != StateReveal {
		c.sendError("Not ready for next round")
		return
	}
	
	// Reset the game state
	c.game.startGame()
}

// Handle request to restart the game
func (c *Client) handleRestartGame() {
	if c.game == nil {
		c.sendError("Not in a game")
		return
	}
	
	c.game.mutex.Lock()
	defer c.game.mutex.Unlock()
	
	// Only the host can restart
	if !c.isHost {
		c.sendError("Only the host can restart the game")
		return
	}
	
	// Reset to lobby state
	c.game.State = StateLobby
	c.game.Round = 0
	c.game.TurnIndex = 0
	c.game.SecretWord = ""
	c.game.ImpostorID = ""
	c.game.Clues = make(map[string][]string)
	c.game.Votes = make(map[string]string)
	
	c.game.broadcastGameState()
}

// Disconnect client from game
func (c *Client) disconnect() {
	if c.game == nil {
		return
	}
	
	c.game.mutex.Lock()
	defer c.game.mutex.Unlock()
	
	// Remove client from game
	player, exists := c.game.Players[c.playerID]
	if exists {
		if c.game.State == StateLobby {
			// If in lobby, fully remove player
			delete(c.game.Players, c.playerID)
			delete(c.game.clients, c.playerID)
			
			// If host left, assign new host
			if c.isHost && len(c.game.Players) > 0 {
				for id, p := range c.game.Players {
					p.IsHost = true
					c.game.clients[id].isHost = true
					break
				}
			}
		} else {
			// If game in progress, mark as inactive
			player.IsActive = false
		}
		
		// Notify other players
		c.game.broadcastPlayerLeft(c.playerID)
		
		// If everyone left or game in progress with too few players
		if len(c.game.Players) == 0 || 
		   (c.game.State != StateLobby && countActivePlayers(c.game) < MinPlayersPerGame) {
			// End game and clean up
			gamesMutex.Lock()
			delete(games, c.game.ID)
			gamesMutex.Unlock()
		} else {
			c.game.broadcastGameState()
		}
	}
	
	c.game = nil
}

// Count active players in a game
func countActivePlayers(game *Game) int {
	count := 0
	for _, p := range game.Players {
		if p.IsActive {
			count++
		}
	}
	return count
}

// Send an error message to client
func (c *Client) sendError(message string) {
	c.sendMessage(Message{
		Type:    TypeError,
		Payload: message,
	})
}

// Send a message to client
func (c *Client) sendMessage(msg Message) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	
	data, err := json.Marshal(msg)
	if err != nil {
		log.Println("Error marshaling message:", err)
		return
	}
	
	select {
	case c.send <- data:
	default:
		log.Println("Client send buffer full")
	}
}

// Broadcast game state to all clients
func (g *Game) broadcastGameState() {
	// Create a safe version of game state (no secret words for broadcast)
	safeState := map[string]interface{}{
		"id":             g.ID,
		"state":          g.State,
		"players":        g.Players,
		"round":          g.Round,
		"turnIndex":      g.TurnIndex,
		"clues":          g.Clues,
		"votes":          g.Votes,
		"timeLimit":      g.TimeLimit,
		"secondsLeft":    int(float64(g.TimeLimit) - time.Since(g.RoundStartTime).Seconds()),
		"impostorId":     "",
		"correctGuesses": 0,
	}
	
	// Only include the impostor ID in the reveal phase
	if g.State == StateReveal {
		safeState["impostorId"] = g.ImpostorID
		
		// Count correct guesses
		correctGuesses := 0
		for voter, votedFor := range g.Votes {
			if voter != g.ImpostorID && votedFor == g.ImpostorID {
				correctGuesses++
			}
		}
		safeState["correctGuesses"] = correctGuesses
	}
	
	msg := Message{
		Type:    TypeGameState,
		Payload: safeState,
	}
	
	for _, client := range g.clients {
		client.sendMessage(msg)
	}
}

// Broadcast player joined notification
func (g *Game) broadcastPlayerJoined(player *Player) {
	msg := Message{
		Type:    TypePlayerJoined,
		Payload: player,
	}
	
	for _, client := range g.clients {
		client.sendMessage(msg)
	}
}

// Broadcast player left notification
func (g *Game) broadcastPlayerLeft(playerID string) {
	msg := Message{
		Type:    TypePlayerLeft,
		Payload: playerID,
	}
	
	for _, client := range g.clients {
		if client.playerID != playerID {
			client.sendMessage(msg)
		}
	}
}