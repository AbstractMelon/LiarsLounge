// Game state variables
let socket;
let gameState = {
    id: null,
    players: {},
    state: 'lobby',
    round: 0,
    turnIndex: 0,
    clues: {},
    votes: {},
    timeLimit: 0,
    secondsLeft: 0
};
let playerInfo = {
    id: null,
    name: '',
    isHost: false,
    isImpostor: false,
    secretWord: '',
    hasVoted: false
};
let timerInterval;

// DOM Elements
const screens = {
    join: document.getElementById('join-screen'),
    lobby: document.getElementById('lobby-screen'),
    game: document.getElementById('game-screen'),
    connectionLost: document.getElementById('connection-lost')
};

// Join Screen Elements
const playerNameInput = document.getElementById('player-name');
const gameCodeInput = document.getElementById('game-code');
const createGameBtn = document.getElementById('create-game-btn');
const joinGameBtn = document.getElementById('join-game-btn');

// Lobby Screen Elements
const displayGameCode = document.getElementById('display-game-code');
const copyCodeBtn = document.getElementById('copy-code-btn');
const playersList = document.getElementById('players-list');
const startGameBtn = document.getElementById('start-game-btn');
const leaveGameBtn = document.getElementById('leave-game-btn');

// Game Screen Elements
const currentRoundDisplay = document.getElementById('current-round');
const timerDisplay = document.getElementById('timer');
const secretWordDisplay = document.getElementById('secret-word');
const roleIndicator = document.getElementById('role-indicator');
const cluesLog = document.getElementById('clues-log');
const turnIndicator = document.getElementById('turn-indicator');
const clueInputContainer = document.getElementById('clue-input-container');
const clueInput = document.getElementById('clue-input');
const submitClueBtn = document.getElementById('submit-clue-btn');
const votingContainer = document.getElementById('voting-container');
const votingOptions = document.getElementById('voting-options');
const submitVoteBtn = document.getElementById('submit-vote-btn');
const resultsContainer = document.getElementById('results-container');
const resultTitle = document.getElementById('result-title');
const resultDetails = document.getElementById('result-details');
const secretWordReveal = document.getElementById('secret-word-reveal');
const nextRoundBtn = document.getElementById('next-round-btn');
const backToLobbyBtn = document.getElementById('back-to-lobby-btn');

// Connection Lost Screen
const reconnectBtn = document.getElementById('reconnect-btn');

// Notification Element
const notification = document.getElementById('notification');

// WebSocket Message Types
const MessageType = {
    JOIN_GAME: 'join_game',
    GAME_STATE: 'game_state',
    START_GAME: 'start_game',
    SUBMIT_CLUE: 'submit_clue',
    SUBMIT_VOTE: 'submit_vote',
    NEXT_ROUND: 'next_round',
    RESTART_GAME: 'restart_game',
    LEAVE_GAME: 'leave_game',
    ERROR: 'error',
    PLAYER_JOINED: 'player_joined',
    PLAYER_LEFT: 'player_left',
    ROLE_ASSIGNMENT: 'role_assignment'
};

// Game States
const GameState = {
    LOBBY: 'lobby',
    ASSIGN_ROLES: 'assign_roles',
    CLUE_ROUND: 'clue_round',
    VOTING: 'voting',
    REVEAL: 'reveal'
};

// Initialize the app
function init() {
    // Add event listeners
    createGameBtn.addEventListener('click', handleCreateGame);
    joinGameBtn.addEventListener('click', handleJoinGame);
    copyCodeBtn.addEventListener('click', handleCopyGameCode);
    startGameBtn.addEventListener('click', handleStartGame);
    leaveGameBtn.addEventListener('click', handleLeaveGame);
    submitClueBtn.addEventListener('click', handleSubmitClue);
    submitVoteBtn.addEventListener('click', handleSubmitVote);
    nextRoundBtn.addEventListener('click', handleNextRound);
    backToLobbyBtn.addEventListener('click', handleBackToLobby);
    reconnectBtn.addEventListener('click', connectWebSocket);

    // Handle enter key in inputs
    playerNameInput.addEventListener('keypress', (e) => {
        if (e.key === 'Enter') {
            handleCreateGame();
        }
    });
    
    gameCodeInput.addEventListener('keypress', (e) => {
        if (e.key === 'Enter') {
            handleJoinGame();
        }
    });
    
    clueInput.addEventListener('keypress', (e) => {
        if (e.key === 'Enter') {
            handleSubmitClue();
        }
    });

    // Force uppercase for game code input
    gameCodeInput.addEventListener('input', () => {
        gameCodeInput.value = gameCodeInput.value.toUpperCase();
    });

    // Connect to WebSocket server
    connectWebSocket();
}

// Connect to WebSocket server
function connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;
    
    // Close existing connection if any
    if (socket) {
        socket.close();
    }
    
    socket = new WebSocket(wsUrl);
    
    socket.onopen = () => {
        console.log('WebSocket connection established');
        showScreen('join');
        
        // Rejoin game if we have game info stored
        const storedGameId = localStorage.getItem('gameId');
        const storedPlayerId = localStorage.getItem('playerId');
        const storedPlayerName = localStorage.getItem('playerName');
        
        if (storedGameId && storedPlayerId && storedPlayerName) {
            playerInfo.id = storedPlayerId;
            playerInfo.name = storedPlayerName;
            gameState.id = storedGameId;
            
            sendMessage({
                type: MessageType.JOIN_GAME,
                payload: {
                    name: playerInfo.name,
                    gameId: gameState.id,
                    playerId: playerInfo.id
                }
            });
        }
    };
    
    socket.onmessage = (event) => {
        const message = JSON.parse(event.data);
        handleServerMessage(message);
    };
    
    socket.onclose = (event) => {
        console.log('WebSocket connection closed:', event.code, event.reason);
        showScreen('connectionLost');
        clearInterval(timerInterval);
    };
    
    socket.onerror = (error) => {
        console.error('WebSocket error:', error);
        showNotification('Connection error', true);
    };
}

// Send message to server
function sendMessage(message) {
    if (socket && socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify(message));
    } else {
        showNotification('Connection lost. Trying to reconnect...', true);
    }
}

// Handle messages from the server
function handleServerMessage(message) {
    console.log('Received message:', message);
    
    switch (message.type) {
        case MessageType.JOIN_GAME:
            handleJoinGameResponse(message.payload);
            break;
        case MessageType.GAME_STATE:
            updateGameState(message.payload);
            break;
        case MessageType.ROLE_ASSIGNMENT:
            handleRoleAssignment(message.payload);
            break;
        case MessageType.PLAYER_JOINED:
            handlePlayerJoined(message.payload);
            break;
        case MessageType.PLAYER_LEFT:
            handlePlayerLeft(message.payload);
            break;
        case MessageType.ERROR:
            showNotification(message.payload, true);
            break;
        default:
            console.warn('Unknown message type:', message.type);
    }
}

// Handle server response to join/create game
function handleJoinGameResponse(payload) {
    playerInfo.id = payload.playerId;
    gameState.id = payload.gameId;
    playerInfo.isHost = payload.isHost;
    
    // Store game info for potential reconnection
    localStorage.setItem('gameId', gameState.id);
    localStorage.setItem('playerId', playerInfo.id);
    localStorage.setItem('playerName', playerInfo.name);
    
    showScreen('lobby');
    displayGameCode.textContent = gameState.id;
    
    showNotification(`${playerInfo.isHost ? 'Created' : 'Joined'} game ${gameState.id}`);
}

// Handle player joined notification
function handlePlayerJoined(player) {
    showNotification(`${player.Name} joined the game`);
}

// Handle player left notification
function handlePlayerLeft(playerId) {
    const playerName = gameState.players[playerId]?.Name || 'A player';
    showNotification(`${playerName} left the game`);
}

// Handle role assignment
function handleRoleAssignment(payload) {
    playerInfo.isImpostor = payload.isImpostor;
    playerInfo.secretWord = payload.secretWord;
    
    // Update UI with role information
    secretWordDisplay.textContent = playerInfo.isImpostor ? '???' : playerInfo.secretWord;
    roleIndicator.textContent = playerInfo.isImpostor ? 'You are the Impostor!' : 'You know the secret word';
    roleIndicator.style.color = playerInfo.isImpostor ? '#e74c3c' : '#27ae60';
}

// Update game state based on server data
function updateGameState(state) {
    // Update local game state
    gameState = {
        ...gameState,
        ...state
    };
    
    // Update UI based on game state
    updatePlayersList();
    
    switch (gameState.state) {
        case GameState.LOBBY:
            showScreen('lobby');
            startGameBtn.style.display = playerInfo.isHost ? 'block' : 'none';
            break;
            
        case GameState.ASSIGN_ROLES:
            showScreen('game');
            hideAllGameElements();
            turnIndicator.textContent = 'Assigning roles...';
            turnIndicator.style.display = 'block';
            break;
            
        case GameState.CLUE_ROUND:
            showScreen('game');
            updateGameUI();
            break;
            
        case GameState.VOTING:
            showScreen('game');
            updateGameUI();
            break;
            
        case GameState.REVEAL:
            showScreen('game');
            updateGameUI();
            break;
    }
}

// Update game UI based on current state
function updateGameUI() {
    hideAllGameElements();
    updateCluesLog();
    
    // Update round information
    currentRoundDisplay.textContent = gameState.round;
    
    // Update timer
    updateTimer(gameState.secondsLeft);
    
    if (gameState.state === GameState.CLUE_ROUND) {
        handleClueRoundUI();
    } else if (gameState.state === GameState.VOTING) {
        handleVotingUI();
    } else if (gameState.state === GameState.REVEAL) {
        handleRevealUI();
    }
}

// Handle UI for clue round
function handleClueRoundUI() {
    turnIndicator.style.display = 'block';
    cluesLog.style.display = 'block';
    
    // Get the active players list
    const activePlayers = Object.values(gameState.players).filter(p => p.IsActive);
    
    // Get current player's turn
    const currentPlayerIndex = gameState.turnIndex < activePlayers.length ? gameState.turnIndex : 0;
    const currentPlayer = activePlayers[currentPlayerIndex];
    
    if (!currentPlayer) return;
    
    if (currentPlayer.ID === playerInfo.id) {
        // It's the current player's turn
        turnIndicator.textContent = "It's your turn! Give a one-word clue.";
        clueInputContainer.style.display = 'flex';
    } else {
        // It's someone else's turn
        turnIndicator.textContent = `Waiting for ${currentPlayer.Name} to give a clue...`;
    }
}

// Handle UI for voting phase
function handleVotingUI() {
    votingContainer.style.display = 'block';
    cluesLog.style.display = 'block';
    
    // Check if player has already voted
    playerInfo.hasVoted = gameState.votes[playerInfo.id] !== undefined;
    
    if (playerInfo.hasVoted) {
        submitVoteBtn.disabled = true;
        submitVoteBtn.textContent = 'Vote Submitted';
    } else {
        submitVoteBtn.disabled = false;
        submitVoteBtn.textContent = 'Submit Vote';
    }
    
    // Populate voting options
    votingOptions.innerHTML = '';
    Object.values(gameState.players)
        .filter(player => player.IsActive && player.ID !== playerInfo.id)
        .forEach(player => {
            const option = document.createElement('div');
            option.className = 'vote-option';
            option.dataset.playerId = player.ID;
            
            if (gameState.votes[playerInfo.id] === player.ID) {
                option.classList.add('selected');
            }
            
            option.textContent = player.Name;
            option.addEventListener('click', () => {
                if (playerInfo.hasVoted) return;
                
                // Remove selected class from all options
                document.querySelectorAll('.vote-option').forEach(opt => {
                    opt.classList.remove('selected');
                });
                
                // Add selected class to this option
                option.classList.add('selected');
            });
            
            votingOptions.appendChild(option);
        });
}

// Handle UI for reveal phase (game results)
function handleRevealUI() {
    resultsContainer.style.display = 'block';
    cluesLog.style.display = 'block';
    
    // Reveal the impostor
    const impostor = gameState.players[gameState.impostorId];
    const impostorName = impostor ? impostor.Name : 'Unknown';
    
    // Check if the impostor was caught
    const correctGuesses = gameState.correctGuesses || 0;
    const totalVoters = Object.keys(gameState.votes).length - (impostor && impostor.IsActive ? 1 : 0);
    const impostorWon = correctGuesses < totalVoters / 2;
    
    if (impostorWon) {
        resultTitle.textContent = 'The Impostor Wins!';
        resultTitle.style.color = '#e74c3c';
    } else {
        resultTitle.textContent = 'The Group Wins!';
        resultTitle.style.color = '#27ae60';
    }
    
    resultDetails.innerHTML = `
        <p>The Impostor was: <strong>${impostorName}</strong></p>
        <p>${correctGuesses} out of ${totalVoters} players guessed correctly.</p>
    `;
    
    secretWordReveal.innerHTML = `The secret word was: <strong>${playerInfo.secretWord || '???'}</strong>`;
    
    // Only host can start next round
    nextRoundBtn.style.display = playerInfo.isHost ? 'block' : 'none';
    backToLobbyBtn.style.display = playerInfo.isHost ? 'block' : 'none';
}

// Update the clues log with current clues
function updateCluesLog() {
    cluesLog.innerHTML = '';
    
    // Loop through rounds and players to show all clues
    for (let round = 1; round <= gameState.round; round++) {
        // Create round header
        const roundHeader = document.createElement('div');
        roundHeader.className = 'round-header';
        roundHeader.textContent = `Round ${round}:`;
        cluesLog.appendChild(roundHeader);
        
        // Show clues for this round
        Object.entries(gameState.clues).forEach(([playerId, clues]) => {
            if (clues.length >= round) {
                const player = gameState.players[playerId];
                if (!player) return;
                
                const clueEntry = document.createElement('div');
                clueEntry.className = 'clue-entry';
                
                const playerNameSpan = document.createElement('span');
                playerNameSpan.className = 'player-name';
                playerNameSpan.textContent = player.Name;
                
                const clueTextSpan = document.createElement('span');
                clueTextSpan.className = 'clue-text';
                clueTextSpan.textContent = clues[round - 1];
                
                // Highlight the impostor's clue in the reveal phase
                if (gameState.state === GameState.REVEAL && playerId === gameState.impostorId) {
                    playerNameSpan.style.color = '#e74c3c';
                }
                
                clueEntry.appendChild(playerNameSpan);
                clueEntry.appendChild(clueTextSpan);
                cluesLog.appendChild(clueEntry);
            }
        });
    }
    
    // Scroll to bottom of clues log
    cluesLog.scrollTop = cluesLog.scrollHeight;
}

// Update the players list in the lobby
function updatePlayersList() {
    playersList.innerHTML = '';
    
    Object.values(gameState.players).forEach(player => {
        const listItem = document.createElement('li');
        listItem.className = 'player-item';
        
        const nameSpan = document.createElement('span');
        nameSpan.className = 'player-name';
        nameSpan.textContent = player.Name;
        
        listItem.appendChild(nameSpan);
        
        if (player.IsHost) {
            const hostBadge = document.createElement('span');
            hostBadge.className = 'host-badge';
            hostBadge.textContent = 'Host';
            listItem.appendChild(hostBadge);
        }
        
        if (!player.IsActive) {
            listItem.style.opacity = '0.5';
            const inactiveBadge = document.createElement('span');
            inactiveBadge.textContent = ' (Disconnected)';
            inactiveBadge.style.color = '#95a5a6';
            nameSpan.appendChild(inactiveBadge);
        }
        
        playersList.appendChild(listItem);
    });
}

// Update timer display and set up interval
function updateTimer(seconds) {
    // Clear existing timer interval
    clearInterval(timerInterval);
    
    // Update timer display
    timerDisplay.textContent = `${seconds}s`;
    
    // Set up timer interval
    timerInterval = setInterval(() => {
        seconds--;
        timerDisplay.textContent = `${seconds}s`;
        
        if (seconds <= 5) {
            timerDisplay.style.color = '#e74c3c';
        } else {
            timerDisplay.style.color = '#2c3e50';
        }
        
        if (seconds <= 0) {
            clearInterval(timerInterval);
        }
    }, 1000);
}

// Hide all game elements
function hideAllGameElements() {
    turnIndicator.style.display = 'none';
    clueInputContainer.style.display = 'none';
    votingContainer.style.display = 'none';
    resultsContainer.style.display = 'none';
    cluesLog.style.display = 'none';
}

// Show a notification
function showNotification(message, isError = false) {
    notification.textContent = message;
    notification.className = 'notification' + (isError ? ' error' : '');
    notification.classList.add('show');
    
    setTimeout(() => {
        notification.classList.remove('show');
    }, 3000);
}

// Show a specific screen
function showScreen(screenName) {
    // Hide all screens
    Object.values(screens).forEach(screen => {
        screen.classList.remove('active');
    });
    
    // Show the requested screen
    screens[screenName].classList.add('active');
}

// Handle creating a new game
function handleCreateGame() {
    const name = playerNameInput.value.trim();
    if (!name) {
        showNotification('Please enter your name', true);
        return;
    }
    
    playerInfo.name = name;
    
    sendMessage({
        type: MessageType.JOIN_GAME,
        payload: {
            name: name,
            createNew: true
        }
    });
}

// Handle joining an existing game
function handleJoinGame() {