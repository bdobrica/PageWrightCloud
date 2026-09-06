import assert from 'node:assert/strict';
import test from 'node:test';
import { readDraft, writeDraft, draftKey, clearDrafts, rememberReturn, consumeReturn, safeReturnPath, isSessionExpiry } from '../src/api/drafts.ts';
import { createSubmissionIdentity } from '../src/api/submission.ts';
import { versionLabel } from '../src/utils/versionLabel.ts';

function storage() {
 const map = new Map();
 return {getItem:k=>map.get(k)??null,setItem:(k,v)=>map.set(k,v),removeItem:k=>map.delete(k),
  get length(){return map.size;},key:i=>[...map.keys()][i]??null};
}
const owner='owner-a', fqdn='one.example.test';
const key='12345678-1234-4123-8123-123456789abc';
test('draft text and clarification survive reload, separated by account and site',()=>{
 const s=storage(), draft={text:'My answer',conversationId:'conversation',question:'Which title?',original:'Edit the title'};
 writeDraft(s,owner,fqdn,draft);
 assert.deepEqual(readDraft(s,owner,fqdn),{...draft,identity:undefined});
 assert.equal(readDraft(s,'owner-b',fqdn).text,'');
 assert.equal(readDraft(s,owner,'two.example.test').text,'');
});
test('uncertain submission restores the exact payload identity before any retry',()=>{
 const s=storage(), payload={fqdn,message:'Edit the title',conversation_id:'conversation'};
 const first=createSubmissionIdentity(()=>key);
 assert.equal(first.begin(payload),key);
 writeDraft(s,owner,fqdn,{text:payload.message,conversationId:payload.conversation_id,identity:first.snapshot()});
 const restored=readDraft(s,owner,fqdn);
 const retry=createSubmissionIdentity(()=>{throw Error('must not create another identity');},restored.identity);
 assert.equal(retry.begin(payload),key);
 assert.equal(retry.begin(payload),null);
 retry.finish('uncertain');
 assert.equal(retry.begin(payload),key);
 retry.finish('success');
 assert.equal(retry.snapshot(),undefined);
 writeDraft(s,owner,fqdn,{text:''});
 assert.equal(readDraft(s,owner,fqdn).identity,undefined);
});
test('invalid and unavailable storage fail explicitly without removing saved evidence',()=>{
 const s=storage();
 for(const raw of ['broken',JSON.stringify({text:'x',identity:{key,fingerprint:'wrong'}}),JSON.stringify({text:4})]) {
  s.setItem(draftKey(owner,fqdn),raw);
  assert.throws(()=>readDraft(s,owner,fqdn));
  assert.equal(s.getItem(draftKey(owner,fqdn)),raw);
 }
 assert.throws(()=>writeDraft({setItem:()=>{throw Error('quota');}},owner,fqdn,{text:'Keep this text'}));
});
test('expiry resumes only the same account and only local allowlisted destinations',()=>{
 const s=storage();
 rememberReturn(s,owner,'/chat/'+fqdn);
 assert.equal(consumeReturn(s,'owner-b'),'/dashboard');
 rememberReturn(s,owner,'/chat/'+fqdn);
 assert.equal(consumeReturn(s,owner),'/chat/'+fqdn);
 assert.equal(consumeReturn(s,owner),'/dashboard');
 for(const path of ['https://evil.test','//evil.test','/chat/../login','/chat/a?token=secret','/chat/%2f%2fevil','/login',null]) assert.equal(safeReturnPath(path),'/dashboard');
 assert.equal(isSessionExpiry(401,'/sites/a/build','Bearer old','old'),true);
 assert.equal(isSessionExpiry(401,'/auth/login','Bearer old','old'),false);
 assert.equal(isSessionExpiry(401,'/sites/a','Bearer old','new'),false);
 assert.equal(isSessionExpiry(403,'/sites/a','Bearer old','old'),false);
 assert.equal(isSessionExpiry(401,'/sites/a',undefined,null),false);
});
test('explicit sign-out removes only draft and return state',()=>{
 const s=storage();
 writeDraft(s,owner,fqdn,{text:'private'});
 writeDraft(s,'owner-b',fqdn,{text:'other'});
 rememberReturn(s,owner,'/chat/'+fqdn);
 s.setItem('unrelated','keep');
 clearDrafts(s);
 assert.equal(s.length,1);
 assert.equal(s.getItem('unrelated'),'keep');
});
test('version labels keep live and preview independent',()=>{
 assert.equal(versionLabel('v1',{live_version_id:'v1',preview_version_id:'v1'}),'Live · Preview');
 assert.equal(versionLabel('v2',{live_version_id:'v1',preview_version_id:'v2'}),'Preview');
 assert.equal(versionLabel('v3',{live_version_id:'v1'}),'Saved build');
 assert.equal(versionLabel('initial'),'Starter source');
});
