import type {ApiRequest,ApiResult,ErrorReceipt,ErrorStatus,Schema} from '../../../../../web/packages/api/src/client.ts';
type Outcome='valid'|'mismatch'|'fatal:policy'|'fatal:configuration'|'fatal:binding';
type Session={revision:string;data:{context:{revision:string}}};
interface Semantics {
  checkReady():Outcome;
  validateSession(value:Session):Outcome;
  validateError(operation:'ReadSession'|'Login',status:ErrorStatus|429,value:ErrorReceipt):Outcome;
}
function typeChecks() {
  // @ts-expect-error async readiness is forbidden
  const a:Semantics['checkReady']=async()=> 'valid' as const;
  // @ts-expect-error ordinary Promise readiness is forbidden
  const b:Semantics['checkReady']=()=>Promise.resolve('valid' as const);
  // @ts-expect-error thenable readiness is forbidden
  const c:Semantics['checkReady']=()=>({then(){}});
  // @ts-expect-error async named hook is forbidden
  const d:Semantics['validateSession']=async()=> 'valid' as const;
  // @ts-expect-error ordinary Promise named hook is forbidden
  const e:Semantics['validateSession']=()=>Promise.resolve('valid' as const);
  // @ts-expect-error thenable named hook is forbidden
  const f:Semantics['validateSession']=()=>({then(){}});
  // @ts-expect-error async operation hook is forbidden
  const g:Semantics['validateError']=async()=> 'valid' as const;
  // @ts-expect-error ordinary Promise operation hook is forbidden
  const h:Semantics['validateError']=()=>Promise.resolve('valid' as const);
  // @ts-expect-error thenable operation hook is forbidden
  const i:Semantics['validateError']=()=>({then(){}});
  // @ts-expect-error an old void hook is not a semantic outcome
  const j:Semantics['validateSession']=()=>{};
  const oldVoid: (value:Session)=>void = async()=>{};
  type NextRequest<T>=ApiRequest<T>&{responseContract?:{numeric:'safe-integers'}};
  const legacy={request:async<T,>(_r:ApiRequest<T>):Promise<ApiResult<T>>=>({kind:'invalid-request'})};
  const permissive:{request<T>(r:NextRequest<T>):Promise<ApiResult<T>>}=legacy;
  // @ts-expect-error the required version marker rejects a legacy client
  const checked:{responseContractVersion:1;request<T>(r:NextRequest<T>):Promise<ApiResult<T>>}=legacy;
  const schema:Schema<Session>={parse(v){return v as Session}};
  void [a,b,c,d,e,f,g,h,i,j,oldVoid,permissive,checked,schema];
}
void typeChecks;
