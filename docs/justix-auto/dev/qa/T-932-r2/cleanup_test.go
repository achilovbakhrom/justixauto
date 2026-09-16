package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// QA keeps the primary socket and cancellation socket separate, and does not
// release cancellation until physical closure and server observations finish.
type qa932Socket struct {
	net.Conn
	mu sync.Mutex
	match string
	armed, lost bool
	closed chan struct{}
	once sync.Once
}
func (w *qa932Socket) Write(p []byte) (int, error) {
	w.mu.Lock()
	if len(p)>5 && p[0]=='Q' && strings.Contains(string(p[5:]),w.match) {w.armed=true}
	w.mu.Unlock()
	return w.Conn.Write(p)
}
func (w *qa932Socket) Read(p []byte) (int,error) {
	n,err:=w.Conn.Read(p)
	w.mu.Lock()
	defer w.mu.Unlock()
	if n>0 && w.armed && !w.lost {w.lost=true;return 0,io.ErrUnexpectedEOF}
	return n,err
}
func (w *qa932Socket) Close() error {
	err:=w.Conn.Close()
	w.once.Do(func(){close(w.closed)})
	return err
}

func TestQA932R2PhysicalClose(t *testing.T) {
	for _, action:=range []string{"poison-lost-lock","ordinary-close-already-native-closed"} {
		t.Run(action,func(t *testing.T){
			f:=startFixture(t); f.bootstrap()
			cfg,err:=pgx.ParseConfig(fmt.Sprintf("host=127.0.0.1 port=%s user=justix_identity password=%s dbname=justix_identity sslmode=disable",f.port,f.password))
			if err!=nil {t.Fatal(err)}
			cfg.DefaultQueryExecMode=pgx.QueryExecModeSimpleProtocol
			release:=make(chan struct{}); started:=make(chan struct{}); var startOnce sync.Once
			defer close(release)
			var mu sync.Mutex
			var wire *qa932Socket
			cfg.DialFunc=func(ctx context.Context,network,address string)(net.Conn,error){
				mu.Lock(); secondary:=wire!=nil; mu.Unlock()
				if secondary {
					startOnce.Do(func(){close(started)})
					select{case <-release:return nil,errors.New("QA cancel dial released");case <-ctx.Done():return nil,ctx.Err()}
				}
				raw,e:=(&net.Dialer{}).DialContext(ctx,network,address);if e!=nil{return nil,e}
				match:="SELECT pg_try_advisory_lock"
				if action!="poison-lost-lock" {match="SELECT 'qa932-lost-close'"}
				mu.Lock();wire=&qa932Socket{Conn:raw,match:match,closed:make(chan struct{})};mu.Unlock()
				return wire,nil
			}
			ctx,cancel:=context.WithTimeout(context.Background(),5*time.Second);defer cancel()
			conn,err:=pgx.ConnectConfig(ctx,cfg);if err!=nil{t.Fatal(err)}
			d,err:=NewDriver(context.Background(),conn,f.config(12,false));if err!=nil{t.Fatal(err)}
			defer d.Close()
			pid:=conn.PgConn().PID()
			begin:=time.Now()
			if action=="poison-lost-lock" {
				if err=d.Lock();!errors.Is(err,ErrPoisoned){t.Fatalf("lost reply not poisoned: %v",err)}
			} else {
				if err=d.Lock();err!=nil{t.Fatal(err)}
				// Fault injection only: directly provoke native asynchronous close
				// after a successful driver lock, then exercise public Driver.Close.
				if _,err=conn.Exec(ctx,"SELECT 'qa932-lost-close'");err==nil{t.Fatal("expected actual reply loss")}
				if !conn.IsClosed(){t.Fatal("native did not enter closed state")}
				if err=d.Close();err!=nil{t.Fatal(err)}
			}
			if elapsed:=time.Since(begin);elapsed>2*time.Second{t.Fatalf("cleanup delayed %s",elapsed)}
			select{case <-started:case <-time.After(time.Second):t.Fatal("async cancel dial not reached")}
			select{case <-conn.PgConn().CleanupDone():t.Fatal("native cleanup finished despite held cancel dial");default:}
			select{case <-wire.closed:default:t.Fatal("physical transport Close has not returned")}
			if _,err=wire.Conn.Write([]byte{'X',0,0,0,4});!errors.Is(err,net.ErrClosed){t.Fatalf("raw owned socket remains writable: %v",err)}
			observer:=f.connect(nil)
			deadline,done:=context.WithTimeout(context.Background(),time.Second);defer done()
			for {
				var count int
				if err=observer.QueryRow(deadline,"SELECT (SELECT count(*) FROM pg_stat_activity WHERE pid=$1)+(SELECT count(*) FROM pg_locks WHERE pid=$1)",pid).Scan(&count);err!=nil{t.Fatal(err)}
				if count==0{break}
				select{case <-deadline.Done():t.Fatalf("server backend/lock survived closure: %d",count);case <-time.After(5*time.Millisecond):}
			}
			var acquired bool
			if err=observer.QueryRow(deadline,"SELECT pg_try_advisory_lock($1)",OwnerLockKey("justix_identity")).Scan(&acquired);err!=nil||!acquired{t.Fatalf("fresh lock failed: %v %v",acquired,err)}
			if _,err=observer.Exec(deadline,"SELECT pg_advisory_unlock($1)",OwnerLockKey("justix_identity"));err!=nil{t.Fatal(err)}
			before:=f.snapshot()
			if !errors.Is(d.Lock(),ErrPoisoned){t.Fatal("closed driver acquired again")}
			if err=d.SetVersion(12,true);err==nil{t.Fatal("closed driver accepted dirty command")}
			if err=d.Run(strings.NewReader("SELECT 1"));err==nil{t.Fatal("closed driver accepted SQL")}
			if err=d.Close();err!=nil{t.Fatal("repeat Close",err)}
			if after:=f.snapshot();after!=before{t.Fatal("closed retries changed retained state")}
			f.retain("qa-r2-closed-no-mutation.json",before)
			t.Logf("physical socket closed, native cleanup held, backend and all locks gone, fresh reacquisition succeeded; action=%s",action)
		})
	}
}

func TestQA932R2OrdinaryCloseRollsBack(t *testing.T) {
	f:=startFixture(t);f.bootstrap()
	conn:=f.connect(nil)
	if _,err:=conn.Exec(context.Background(),"BEGIN; CREATE TABLE public.qa932_uncommitted(id int); SELECT pg_advisory_lock(177932)");err!=nil{t.Fatal(err)}
	d,err:=NewDriver(context.Background(),conn,f.config(12,false));if err!=nil{t.Fatal(err)}
	pid:=conn.PgConn().PID()
	if err=d.Close();err!=nil{t.Fatal(err)}
	observer:=f.connect(nil);observeBackendExit(t,observer,pid)
	if got:=f.must("SELECT to_regclass('public.qa932_uncommitted') IS NULL");got!="t"{t.Fatal("uncommitted DDL survived",got)}
	var acquired bool
	if err=observer.QueryRow(context.Background(),"SELECT pg_try_advisory_lock(177932)").Scan(&acquired);err!=nil||!acquired{t.Fatalf("ordinary Close retained session lock: %v %v",acquired,err)}
	if _,err=observer.Exec(context.Background(),"SELECT pg_advisory_unlock(177932)");err!=nil{t.Fatal(err)}
	if err=d.Close();err!=nil{t.Fatal(err)}
	f.retain("qa-r2-normal-close.json",f.snapshot())
}
