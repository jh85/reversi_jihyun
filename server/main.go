package main

import (
	"bytes"
	"sort"
	"strconv"
	"strings"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"sync"
	"time"

	"context"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"game"
)

type Move string

type BufferTO struct {
	buf  []byte
	off  int
	conn net.Conn
	bufsize int
}

func NewReaderTO(conn net.Conn, bufsz int) *BufferTO {
	b := BufferTO{
		buf:  make([]byte, bufsz, bufsz),
		off:  0,
		conn: conn,
		bufsize: bufsz,
	}
	return &b
}

func (bt *BufferTO) ReadBytesTO(delim byte, timeout_msec int) (line []byte, err error) {
	t := time.Now().Add(time.Duration(timeout_msec) * time.Millisecond)
	for {
		i := bytes.IndexByte(bt.buf[:bt.off], delim)
		if i < 0 {
			if bt.off == bt.bufsize {
				new_buf := make([]byte, bt.bufsize*2, bt.bufsize*2)
				copy(new_buf, bt.buf)
				bt.buf = new_buf
				bt.bufsize *= 2
			}
			bt.conn.SetReadDeadline(t)
			cnt, err := bt.conn.Read(bt.buf[bt.off:])
			if err != nil {
				bt.buf = make([]byte, bt.bufsize, bt.bufsize)
				bt.off = 0
				return []byte{}, err
			}
			bt.off += cnt
		} else {
			line = make([]byte, i+1, i+1)
			copy(line, bt.buf[:i+1])
			copy(bt.buf, bt.buf[i+1:bt.off])
			bt.off = bt.off - i - 1
			return line, nil
		}
	}
}

func (u *User) ReadlineTO(timeout_msec int) ([]byte,error) {
	return u.Rbuf.ReadBytesTO(byte('\n'), timeout_msec)
}

func (u *User) Writeline(line []byte) (int,error) {
	return u.Conn.Write(line)
}

type UserState int

const (
	logout UserState = iota
	login
	chaperone
	ready
	playing
)

type User struct {
	Userid      string
	Conn        net.Conn
	Rbuf        *BufferTO
	Login_time  int64
	Chaperone_time int64
	Remote_addr string
	State       UserState
	Statistics  *UserStatistics
}

type UserStatistics struct {
	rating float64
	n_win int
	n_loss int
	n_draw int
	n_illegalmove int
	n_timeout int
}

type Database struct {
	queue []interface{}
	mu sync.Mutex
}

type Lobby struct {
	queue map[string]*User
	mu    sync.Mutex
}

func create_user(userid string, conn net.Conn) *User {
	ustat := &UserStatistics{
		rating: 1500.0,
		n_win: 0,
		n_loss: 0,
		n_draw: 0,
		n_illegalmove: 0,
		n_timeout: 0,
	}
	u := User{
		Userid:      userid,
		Conn:        conn,
		Rbuf:        NewReaderTO(conn, 8192),
		Login_time:  time.Now().Unix(),
		Chaperone_time: 0,
		Remote_addr: conn.RemoteAddr().String(),
		State:       logout,
		Statistics:  ustat,
	}
	return &u
}

func main() {
	rand.Seed(time.Now().UnixNano())
	addr := flag.String("addr", ":19714", "server IP address:port")
	boardlen := flag.Int("boardlen", 8, "reversi board side length")
	timeout_msec := flag.Int("timeout", 10000, "timeout in msec")
	dbuse := flag.Bool("db", false, "use database to store game info")
	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Println("Listen error ln =", ln, " err =", err)
		return
	}

	lobby := &Lobby{
		queue: make(map[string]*User),
	}

	var db *Database = nil
	if *dbuse == true {
		db = &Database{
			queue: make([]interface{},0,0),
		}
		go func(mydb *Database) {
			uri := "mongodb://127.0.0.1:27017"
			client,err := mongo.NewClient(options.Client().ApplyURI(uri))
			if err != nil {
				log.Println(err)
				return
			}
			ctx := context.Background()
			err = client.Connect(ctx)
			if err != nil {
				log.Println(err)
				return
			}
			defer client.Disconnect(ctx)

			col := client.Database("reversi").Collection("games")
			for {
				err = nil
				mydb.mu.Lock()
				l := len(mydb.queue)
				if l > 0 {
					_,err = col.InsertMany(ctx, mydb.queue)
					mydb.queue = make([]interface{},0,0)
				}
				mydb.mu.Unlock()
				if err != nil {
					log.Println("game data insert failure err =", err)
				} else {
					log.Println("game data inserted =", l)
				}
				time.Sleep(time.Duration(10) * time.Second)
			}
		}(db)
	}
	
	go func(mylobby *Lobby) {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Println("Accept error", conn, err)
				time.Sleep(time.Duration(10) * time.Second)
				continue
			}
			user, err := do_login_add_to_lobby(conn, mylobby)
			if err != nil {
				log.Println("login failed userid =", user.Userid,
					" RemoteAddr =", user.Remote_addr,
					" err =", err)
				send_logout(user, err)
				conn.Close()
				continue
			}
			log.Println("new login userid =", user.Userid, " RemoteAddr =", user.Remote_addr)
		}
	}(lobby)

	log_freq := 0
	for {
		stats := []int{0,0,0,0,0}
		rusers := []*User{}
		lobby.mu.Lock()
		for _,u := range lobby.queue {
			if u.State == logout {
				stats[logout] += 1
			} else if u.State == login {
				stats[login] += 1
				u.State = chaperone
				go do_chaperone(u, lobby)
			} else if u.State == chaperone {
				stats[chaperone] += 1
			} else if u.State == ready {
				stats[ready] += 1
				rusers = append(rusers, u)
			} else if u.State == playing {
				stats[playing] += 1
			}
		}
		lobby.mu.Unlock()

		num_rusers := len(rusers)
		if num_rusers > 1 {
			if num_rusers % 2 == 1 {
				sort.Slice(rusers, func(i,j int) bool {
					return rusers[i].Chaperone_time < rusers[j].Chaperone_time
				})
				num_rusers--
			}
			rand.Shuffle(num_rusers, func(i,j int) {
				rusers[i],rusers[j] = rusers[j],rusers[i]
			})
			for i := 0; i < num_rusers; i += 2 {
				u0 := rusers[i]
				u1 := rusers[i+1]
				u0.State = playing
				u1.State = playing
				go do_game(u0, u1, lobby, *boardlen, *timeout_msec, db)
			}
		}
		if log_freq > 10 {
			log.Println("Logout:", stats[0], " Login:", stats[1], " Chaperone:", stats[2],
				" Ready:", stats[3], " Playing:", stats[4])
			log_freq = 0
		}
		log_freq++
		time.Sleep(time.Duration(2) * time.Second)
	}
}

func do_chaperone(u *User, l *Lobby) {
	isready_msg := IsReady{
		Message: "ISREADY",
	}
	j := str2json(&isready_msg)
	u.Writeline(j)
	
	line,err := u.ReadlineTO(1000*10)
	if err != nil {
		u.Logout(l,err)
		log.Println("chaperone: ReadlineTO failed err =", err)
		return
	}
	var r UserMessage
	err = json.Unmarshal(line, &r)
	if err != nil {
		u.Logout(l,err)
		log.Println("chaperone: Unmarshal failed", line, err)
		return
	}
	if strings.ToUpper(string(r.Message)) != "READY" {
		u.Logout(l,errors.New("wrong READY"))
		log.Println("chaperone: wrong READY message", string(line), " userid=", u.Userid)
		return
	}
	u.Chaperone_time = time.Now().UnixNano()
	u.State = ready
}

func (u *User) Logout(l *Lobby, err error) {
	u.State = logout
	l.mu.Lock()
	delete(l.queue, u.Userid)
	l.mu.Unlock()
	log.Println("Logout: userid =", u.Userid, " err =", err.Error())
}	

func mk_game(b *game.Board, u0 *User, u1 *User, timeout_msec int) *Game {
	gs := &GameState{
		s: Playing,
		m: "",
	}
	g := &Game{
		Gameid: gen_game_id(),
		StartTime: time.Now().Unix(),
		EndTime: 0,
		Black: u0,
		White: u1,
		Moves: []Move{},
		Timeout: timeout_msec,
		State: gs,
		Board: b,
	}
	return g
}

func do_game(u0 *User, u1 *User, l *Lobby, boardlen int, timeout_msec int, db *Database) {
	sfen := game.MakeInitialSFEN(boardlen)
	b := game.NewBoardSFEN(boardlen, sfen)
	g := mk_game(b,u0,u1,timeout_msec)
	for {
		g = send_req_get_resp(u0,g)
		if g.is_gameover() {
			break
		}
		g = send_req_get_resp(u1,g)
		if g.is_gameover() {
			break
		}
	}

	if db != nil {
		gm := g2gm(g)
		db.mu.Lock()
		db.queue = append(db.queue, &gm)
		db.mu.Unlock()
	}

	// update statistics and ratings
	if g.State.s == BlackWin {
		K := 32.0
		W := 1.0 / (math.Pow(10.0, (u0.Statistics.rating - u1.Statistics.rating)/400.0) + 1.0)
		//W := u1.Statistics.rating-u0.Statistics.rating) / 800.0 + 0.5
		u0.Statistics.rating += K * W
		u1.Statistics.rating -= K * W
		u0.Statistics.n_win++
		u1.Statistics.n_loss++
	} else if g.State.s == WhiteWin {
		K := 32.0
		W := 1.0 / (math.Pow(10.0, (u1.Statistics.rating - u0.Statistics.rating)/400.0) + 1.0)
		//W := u0.Statistics.rating-u1.Statistics.rating) / 800.0 + 0.5
		u0.Statistics.rating -= K * W
		u1.Statistics.rating += K * W
		u0.Statistics.n_loss++
		u1.Statistics.n_win++
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go send_result(u0,g,l,&wg)
	go send_result(u1,g,l,&wg)
	wg.Wait()
}

func send_result(u *User, g *Game, l *Lobby, wg *sync.WaitGroup) {
	defer wg.Done()

	j := game2result(g)
	u.Writeline(j)

	n_wrong_msg := 0
	for {
		line,err := u.ReadlineTO(g.Timeout)
		if err != nil {
			u.Logout(l, err)
			log.Println("send_result: ReadlineTO failed:", string(line), err)
			return
		}

		var r UserMessage
		err = json.Unmarshal(line, &r)
		if err != nil {
			u.Logout(l, err)
			log.Println("send_result: Unmarshal failed:", string(line), err)
			return
		}
		if strings.ToUpper(string(r.Message)) == "RESULTOK" {
			break
		} else {
			if n_wrong_msg == 0 {
				// user may send one Move when we are waiting for RESULTOK
				n_wrong_msg += 1
			} else {
				u.Logout(l, errors.New("wrong RESULTOK"))
				log.Println("send_result: wrong RESULTOK message:", string(line), err)
				return
			}
		}
	}
	u.State = login
}

func send_req_get_resp(u *User, g *Game) *Game {
	j := game2play(g)
	u.Writeline(j)

	b := g.Board
	j,err := u.ReadlineTO(g.Timeout)
	if err != nil {
		if b.IsBlackTurn() {
			g.State.s = BlackTimeout
		} else {
			g.State.s = WhiteTimeout
		}
		return g
	}
	m := json2msg(j)
	pos,mv,err := g.str2move(m.Message)
	if err != nil {
		//log.Println("send_req_get_resp: str2move failure pos =", pos,
		//	" mv =", mv, " err =", err, " m.Message =", m.Message,
		//	" gameid =", g.Gameid, " userid =", u.Userid)
		g.State.m = m.Message
		if b.IsBlackTurn() {
			g.State.s = BlackIllegalMove
		} else {
			g.State.s = WhiteIllegalMove
		}
		return g
	}
	g.Moves = append(g.Moves, mv)

	err = g.check_and_move(pos)
	if err != nil {
		//log.Println("send_req_get_resp: check_and_move failure pos =", pos,
		//	" err =", err, " m.Message =", m.Message,
		//	" gameid =", g.Gameid, " userid =", u.Userid)
		g.State.m = m.Message
		if b.IsBlackTurn() {
			g.State.s = BlackIllegalMove
		} else {
			g.State.s = WhiteIllegalMove
		}
		return g
	}

	if b.IsGameOver() {
		n_black := b.CountBlack()
		n_white := b.CountWhite()
		if n_black > n_white {
			g.State.s = BlackWin
		} else if n_black == n_white {
			g.State.s = Draw
		} else {
			g.State.s = WhiteWin
		}
		g.State.m = fmt.Sprintf("%d/%d", n_black, n_white)
	}
	return g
}

func (g *Game) is_gameover() bool {
	return g.State.s != Playing
}

func do_login_add_to_lobby(conn net.Conn, lb *Lobby) (*User, error) {
	u := create_user("", conn)
	line,err := u.ReadlineTO(10000)
	if err != nil {
		log.Println("do_login: ReadlineTO failed", line, err)
		if os.IsTimeout(err) {
			return u, errors.New("login timeout")
		} else {
			return u, err
		}
	}
	var l Login
	err = json.Unmarshal(line, &l)
	if err != nil {
		log.Println("do_login: Login failed", line, err)
		return u, errors.New("broken login message")
	}
	// sanity check
	if l.Message != "LOGIN" || len(l.Userid) == 0 || len(l.Password) == 0 {
		log.Println("do_login: Login failed 2 ", line, err)
		return u, errors.New("failed login attempt")
	}

	u.Userid = l.Userid
	u.State = login
	lb.mu.Lock()
	if _, ok := lb.queue[u.Userid]; ok {
		lb.mu.Unlock()
		u.State = logout
		log.Println("do_login: userid =", u.Userid, " already exists")
		return u, errors.New("duplicate login")
	} else {
		// TODO password check
		lb.queue[u.Userid] = u
		lb.mu.Unlock()
		return u, nil
	}
}

func send_logout(u *User, err error) {
	logout := Logout{
		Message: "LOGOUT",
		Reason:  err.Error(),
	}
	j := str2json(&logout)
	u.Writeline(j)
}

func game2play(g *Game) []byte {
	b := g.Board
	turn := []string{"black", "white"}[b.Turn]
	moves := []Move{}
	if len(g.Moves) != 0 {
		moves = []Move{g.Moves[len(g.Moves)-1]}
	}
	m := GameMessage{
		Message: "PLAY",
		Gameid: g.Gameid,
		StartTime: g.StartTime,
		EndTime: g.EndTime,
		Black: g.Black.Userid,
		BlackRating: strconv.Itoa(int(g.Black.Statistics.rating)),
		White: g.White.Userid,
		WhiteRating: strconv.Itoa(int(g.White.Statistics.rating)),
		Turn: turn,
		Position: b.ToSFEN(),
		Moves: moves,
		BoardSize: b.Boardlen,
		Timeout: g.Timeout,
		State: gamestate2str(g),
	}
	return str2json(m)
}

func game2result(g *Game) []byte {
	b := g.Board
	turn := []string{"black", "white"}[b.Turn]
	m := GameMessage{
		Message: "RESULT",
		Gameid: g.Gameid,
		StartTime: g.StartTime,
		EndTime: time.Now().Unix(),
		Black: g.Black.Userid,
		BlackRating: strconv.Itoa(int(g.Black.Statistics.rating)),
		White: g.White.Userid,
		WhiteRating: strconv.Itoa(int(g.White.Statistics.rating)),
		Turn: turn,
		Position: b.ToSFEN(),
		Moves: g.Moves,
		BoardSize: b.Boardlen,
		Timeout: g.Timeout,
		State: gamestate2str(g),
	}
	return str2json(m)
}

func g2gm(g *Game) *GameMessage {
	b := g.Board
	turn := []string{"black", "white"}[b.Turn]
	m := GameMessage{
		Message: "RESULT",
		Gameid: g.Gameid,
		StartTime: g.StartTime,
		EndTime: time.Now().Unix(),
		Black: g.Black.Userid,
		BlackRating: strconv.Itoa(int(g.Black.Statistics.rating)),
		White: g.White.Userid,
		WhiteRating: strconv.Itoa(int(g.White.Statistics.rating)),
		Turn: turn,
		Position: b.ToSFEN(),
		Moves: g.Moves,
		BoardSize: b.Boardlen,
		Timeout: g.Timeout,
		State: gamestate2str(g),
	}
	return &m
}

func str2json(v any) []byte {
	b, _ := json.Marshal(v)
	b = append(b, byte('\n'))
	return b
}

func json2msg(b []byte) UserMessage {
	var m UserMessage
	json.Unmarshal(b, &m)
	return m
}

func (g *Game) check_and_move(pos int) error {
	b := g.Board
	if b.IsLegalMove(game.Position(pos)) {
		b.MoveUpdate(game.Position(pos))
		return nil
	} else {
		return errors.New("illegal move")
	}
}

func (g *Game) check_and_move_old(pos int) error {
	b := g.Board
	lms := b.LegalMoves()
	if len(lms) == 0 {
		if pos == -1 {
			b.MoveUpdate(game.Position(pos))
			return nil
		} else {
			return errors.New("illegal move")
		}
	} else {
		for _,mv := range lms {
			if mv == game.Position(pos) {
				b.MoveUpdate(game.Position(pos))
				return nil
			}
		}
		return errors.New("illegal move")
	}
}

func letter2int(c byte) int {
	return int(c - byte('a'))
}

func (g *Game) str2move(msg string) (int,Move,error) {
	move := strings.ToLower(strings.TrimSpace(msg))
	length := len(move)
	if length < 2 {
		return 0,"",errors.New("empty move")
	}
	
	if move == "pass" {
		return -1,"pass",nil
	}
	
	if !game.IsLetter(move[0]) {
		return 0,Move(move),errors.New("wrong format move")
	}
	var x,y int
	var err error = nil
	if game.IsLetter(move[1]) {
		digit0 := letter2int(move[0])
		digit1 := letter2int(move[1])
		y = digit0*26 + digit1
		x,err = strconv.Atoi(move[2:length])
	} else {
		y = letter2int(move[0])
		x,err = strconv.Atoi(move[1:length])
	}
	if err != nil {
		return 0,Move(move),err
	}
	pos := (x-1) * g.Board.Boardlen + y
	return pos,Move(move),nil
}

// 4 messages from users
type Login struct {
	Message  string // LOGIN
	Userid   string
	Password string
}

type UserMessage struct {
	Message string // READY, a6,A6, pass,PASS, LOGOUT
}

// messagers from server
type Logout struct {
	Message string // LOGIN
	Reason  string // OK, WRONG PASSWORD, DUPLICATE LOGIN ATTEMPT
}

type IsReady struct {
	Message string
}

type GameStateCode int

const (
	Playing GameStateCode = iota
	BlackWin
	WhiteWin
	BlackIllegalMove
	WhiteIllegalMove
	BlackTimeout
	WhiteTimeout
	BlackDisconnected
	WhiteDisconnected
	Draw
)

type GameState struct {
	s GameStateCode
	m string
}

func gamestate2str(g *Game) string {
	msg_list := []string{
		"playing",
		"black win",
		"white win",
		"black illegal move",
		"white illegal move",
		"black timeout",
		"white timeout",
		"black disconnected",
		"white disconnected",
		"draw",
	}
	if g.State.s == BlackWin || g.State.s == WhiteWin || g.State.s == Draw {
		return msg_list[int(g.State.s)] + " " + g.State.m
	} else if g.State.s == BlackIllegalMove || g.State.s == WhiteIllegalMove {
		return msg_list[int(g.State.s)] + " \"" + g.State.m + "\""
	} else {
		return msg_list[int(g.State.s)]
	}
}

type Game struct {
	Gameid     string
	StartTime  int64
	EndTime    int64
	Black      *User
	White      *User
	Moves      []Move
	Timeout    int
	State      *GameState
	Board      *game.Board //pointer to Board
}

type GameMessage struct {
	Message     string // PLAY, RESULT
	Gameid      string
	StartTime   int64
	EndTime     int64
	Black       string
	BlackRating string
	White       string
	WhiteRating string
	Turn        string
	Position    string
	Moves       []Move
	BoardSize   int
	Timeout     int
	State       string
}

func gen_game_id() string {
	t := time.Now().UnixNano()
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz")
	b := make([]rune, 10)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return fmt.Sprintf("game-%d-%s", t, string(b))
}
