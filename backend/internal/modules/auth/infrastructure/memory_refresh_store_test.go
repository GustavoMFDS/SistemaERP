package infrastructure

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDevRefreshTokensAreSingleUse(t *testing.T) {
	store:=NewMemoryRefreshTokenStore()
	ctx:=context.Background()
	if err:=store.Save(ctx,"test-token-1","user-a",60);err!=nil{t.Fatal(err)}
	ok,err:=store.Consume(ctx,"test-token-1","user-a")
	if err!=nil||!ok{t.Fatalf("first use: %t %v",ok,err)}
	ok,err=store.Consume(ctx,"test-token-1","user-a")
	if err!=nil||ok{t.Fatalf("refresh replay accepted: %t %v",ok,err)}
	if err:=store.Save(ctx,"test-token-2","user-b",60);err!=nil{t.Fatal(err)}
	if err:=store.Revoke(ctx,"test-token-2");err!=nil{t.Fatal(err)}
	ok,err=store.Consume(ctx,"test-token-2","user-b")
	if err!=nil||ok{t.Fatalf("revoked token accepted: %t %v",ok,err)}
	if err:=store.Save(ctx,"test-token-3","user-a",1);err!=nil{t.Fatal(err)}
	store.mu.Lock()
	entry:=store.items["test-token-3"]
	entry.expiresAt=time.Now().Add(-time.Second)
	store.items["test-token-3"]=entry
	store.mu.Unlock()
	ok,err=store.Consume(ctx,"test-token-3","user-a")
	if err!=nil||ok{t.Fatalf("expired token accepted: %t %v",ok,err)}
}

func TestDevRefreshConcurrentConsumptionOnlyOnce(t *testing.T) {
	store:=NewMemoryRefreshTokenStore()
	ctx:=context.Background()
	if err:=store.Save(ctx,"concurrent-jti","user-a",60);err!=nil{t.Fatal(err)}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i:=0;i<50;i++{
		wg.Add(1)
		go func(){
			defer wg.Done()
			ok,err:=store.Consume(ctx,"concurrent-jti","user-a")
			if err==nil&&ok{successes.Add(1)}
		}()
	}
	wg.Wait()
	if got:=successes.Load();got!=1 {
		t.Fatalf("single-use rotation violated: %d accepted",got)
	}
}

func TestDevRefreshRejectsInvalidAndCanceledInputs(t *testing.T) {
	store:=NewMemoryRefreshTokenStore()
	ctx,cancel:=context.WithCancel(context.Background())
	cancel()
	if err:=store.Save(ctx,"jti","user",60);err==nil{
		t.Fatal("canceled context must not save")
	}
	if err:=store.Save(context.Background(),"","user",60);err==nil{
		t.Fatal("missing JTI must not save")
	}
	if err:=store.Save(context.Background(),"jti","user",60);err!=nil{t.Fatal(err)}
	if err:=store.Save(context.Background(),"jti","user",60);err==nil{
		t.Fatal("duplicate JTI must be refused")
	}
}
