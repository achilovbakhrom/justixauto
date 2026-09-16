package postgres_test

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "os"
 "testing"
 "github.com/google/uuid"
 driver "gorm.io/driver/postgres"
 "gorm.io/gorm"
 "gorm.io/gorm/logger"
 "justixauto/pkg/persistence"
 identity "justixauto/services/identity/adapter/postgres"
)

var qaQueryFault=errors.New("independent checker transaction fault")
type qaPool struct { *sql.DB; failAt,queries,begins,commits,rollbacks int }
func(p *qaPool)BeginTx(ctx context.Context,opts *sql.TxOptions)(gorm.ConnPool,error){
 if opts==nil||opts.Isolation!=sql.LevelReadCommitted{return nil,errors.New("not actual ReadCommitted")}
 p.begins++;tx,err:=p.DB.BeginTx(ctx,opts);if err!=nil{return nil,err};return &qaTx{Tx:tx,p:p},nil
}
type qaTx struct{*sql.Tx;p *qaPool}
func(tx *qaTx)QueryContext(ctx context.Context,q string,args ...any)(*sql.Rows,error){tx.p.queries++;if tx.p.queries==tx.p.failAt{return nil,qaQueryFault};return tx.Tx.QueryContext(ctx,q,args...)}
func(tx *qaTx)Commit()error{tx.p.commits++;return tx.Tx.Commit()}
func(tx *qaTx)Rollback()error{tx.p.rollbacks++;return tx.Tx.Rollback()}

func TestQA931Independent(t *testing.T){
 if os.Getenv("JUSTIXAUTO_TEST_IDENTITY_MECHANICS")!="1"{t.Skip("explicit owned fixture required")}
 f:=start(t);profile,_:=f.compatibleInstallation();db:=f.open("justix_identity_runtime");ctx:=context.Background()
 baseline:=f.compatibilitySnapshot()
 pool,err:=db.DB();if err!=nil{t.Fatal(err)}
 for _,failAt:=range []int{1,5}{t.Run(fmt.Sprintf("transaction-check-query-%d-fails-before-bind",failAt),func(t *testing.T){
  p:=&qaPool{DB:pool,failAt:failAt};handle,err:=gorm.Open(driver.New(driver.Config{Conn:p}),&gorm.Config{Logger:logger.Default.LogMode(logger.Silent)});if err!=nil{t.Fatal(err)}
  binds,calls:=0,0;r,err:=identity.NewCompatibleUnitOfWork(handle,profile,func(tx *gorm.DB)(*gorm.DB,error){binds++;return tx,nil});if err!=nil{t.Fatal(err)}
  if err:=r.Check(ctx);err!=nil{t.Fatalf("pool startup should succeed: %v",err)}
  err=r.Run(ctx,func(*gorm.DB)error{calls++;return nil})
  if !errors.Is(err,qaQueryFault)||!errors.Is(err,persistence.ErrIncompatible)||!errors.Is(err,identity.ErrIncompatiblePersistence)||binds!=0||calls!=0||p.begins!=1||p.commits!=0||p.rollbacks!=1||p.queries!=failAt{t.Fatalf("err=%v binds=%d calls=%d begin=%d commit=%d rollback=%d queries=%d",err,binds,calls,p.begins,p.commits,p.rollbacks,p.queries)}
  p.failAt=0
  if err:=r.Run(ctx,func(*gorm.DB)error{calls++;return nil});err!=nil||binds!=1||calls!=1||p.commits!=1{t.Fatalf("fresh subsequent run failed: %v",err)}
 })}
 t.Run("nil-callback-opens-no-transaction",func(t *testing.T){p:=&qaPool{DB:pool};handle,err:=gorm.Open(driver.New(driver.Config{Conn:p}),&gorm.Config{Logger:logger.Default.LogMode(logger.Silent)});if err!=nil{t.Fatal(err)};r,err:=identity.NewCompatibleUnitOfWork(handle,profile,func(tx *gorm.DB)(*gorm.DB,error){t.Fatal("nil callback bound");return tx,nil});if err!=nil{t.Fatal(err)};if r.Run(ctx,nil)==nil||p.begins!=0{t.Fatal("nil callback reached SQL transaction")}})
 t.Run("factory-write-error-rolls-back",func(t *testing.T){
  id:=uuid.NewString();stop:=errors.New("independent bind error");calls:=0
  r,err:=identity.NewCompatibleUnitOfWork(db,profile,func(tx *gorm.DB)(*gorm.DB,error){if err:=tx.Exec("INSERT INTO synthetic_feature.items(id,body) VALUES(?::uuid,'factory candidate')",id).Error;err!=nil{return nil,err};return nil,stop});if err!=nil{t.Fatal(err)}
  if err:=r.Run(ctx,func(*gorm.DB)error{calls++;return nil});!errors.Is(err,stop)||calls!=0{t.Fatalf("err=%v calls=%d",err,calls)}
  if n:=f.must("justix_identity","justix_identity_runtime","SELECT count(*) FROM synthetic_feature.items WHERE id='"+id+"'");n!="0"{t.Fatal("factory write committed")}
 })
 t.Run("callback-panic-rolls-back",func(t *testing.T){
  id:=uuid.NewString();r,err:=identity.NewCompatibleUnitOfWork(db,profile,func(tx *gorm.DB)(*gorm.DB,error){return tx,nil});if err!=nil{t.Fatal(err)}
  func(){defer func(){if recover()!="qa931 panic"{t.Error("panic swallowed or changed")}}();_ =r.Run(ctx,func(tx *gorm.DB)error{if err:=tx.Exec("INSERT INTO synthetic_feature.items(id,body) VALUES(?::uuid,'panic candidate')",id).Error;err!=nil{t.Fatal(err)};panic("qa931 panic")})}()
  if n:=f.must("justix_identity","justix_identity_runtime","SELECT count(*) FROM synthetic_feature.items WHERE id='"+id+"'");n!="0"{t.Fatal("panic write committed")}
 })
 t.Run("active-and-ended-handle-no-fallback",func(t *testing.T){
  tx:=db.Begin(&sql.TxOptions{Isolation:sql.LevelReadCommitted});if tx.Error!=nil{t.Fatal(tx.Error)};defer tx.Rollback()
  binds,calls:=0,0;r,err:=identity.NewCompatibleUnitOfWork(tx,profile,func(tx *gorm.DB)(*gorm.DB,error){binds++;return tx,nil});if err!=nil{t.Fatal(err)}
  if err:=r.Check(ctx);err!=nil{t.Fatal(err)}
  if err:=r.Run(ctx,func(*gorm.DB)error{calls++;return nil});err==nil||binds!=0||calls!=0{t.Fatal("nested Run borrowed active transaction")}
  if err:=tx.Rollback().Error;err!=nil{t.Fatal(err)}
  if err:=r.Check(ctx);!errors.Is(err,identity.ErrIncompatiblePersistence)||!errors.Is(err,sql.ErrTxDone){t.Fatalf("ended check: %v",err)}
  if err:=r.Run(ctx,func(*gorm.DB)error{calls++;return nil});err==nil||binds!=0||calls!=0{t.Fatal("ended Run fell back to pool")}
 })
 t.Run("constructor-profile-isolation",func(t *testing.T){
  good,err:=identity.NewCompatibleUnitOfWork(db,profile,func(tx *gorm.DB)(*gorm.DB,error){return tx,nil});if err!=nil{t.Fatal(err)}
  spec:=profile.Specification();spec.Baseline.ApprovalRef="independent wrong approval";bad,err:=identity.NewCompatibleUnitOfWork(db,mustProfile(t,spec),func(tx *gorm.DB)(*gorm.DB,error){t.Fatal("wrong profile bound");return tx,nil});if err!=nil{t.Fatal(err)}
  for range 2{if err:=good.Run(ctx,func(*gorm.DB)error{return nil});err!=nil{t.Fatal(err)};if err:=bad.Check(ctx);!errors.Is(err,identity.ErrIncompatiblePersistence){t.Fatalf("profile sharing: %v",err)}}
 })
 if after:=f.compatibilitySnapshot();after!=baseline{t.Fatal("independent error paths changed retained fixture state")}
 t.Log("all independent failure paths preserved exact original retained snapshot")
}
