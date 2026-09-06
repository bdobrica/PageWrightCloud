import assert from 'node:assert/strict';
import test from 'node:test';
import { startJobPolling, mergeJob, buildFailureDetail, POLL_MAX_ROUNDS, POLL_MAX_ELAPSED } from '../src/api/jobPolling.ts';

const job = (status = 'pending', extra = {}) => ({ job_id:'job', site_id:'site', source_version:'initial', target_version:'version', status, dispatch_state:'accepted', created_at:'2026-09-06T12:00:00Z', updated_at:'2026-09-06T12:00:01Z', ...extra });
const page = jobs => ({data:jobs,page:1,page_size:25,total_count:jobs.length,total_pages:jobs.length ? 1 : 0});
const flush = async () => { for (let i=0;i<20;i++) await Promise.resolve(); };
function harness(overrides = {}) {
 const states=[], delays=[], timers=[];
 let completions=0, requests=0, time=0;
 const stop = startJobPolling({
  loadSite:async()=>({id:'site'}), loadPage:async()=>page([job()]),
  loadJob:async()=>{ requests++; return job(); },
  onState:state=>states.push(state), onCompleted:()=>completions++,
  now:()=>time,
  schedule:(fn,delay)=>{const timer={fn,cancelled:false}; timers.push(timer); delays.push(delay); return()=>{timer.cancelled=true;};},
  ...overrides,
 });
 return {states,delays,timers,stop, get completions(){return completions;},get requests(){return requests;},
  advanceTime(ms){time+=ms;},
  async next(){const t=timers.shift(); assert.ok(t,'expected scheduled check'); if(!t.cancelled)t.fn(); await flush();},
 };
}

test('pending → running → completed refreshes versions once and stops',async()=>{
 const updates=[job('running'),job('completed')];
 const h=harness({loadJob:async()=>updates.shift()}); await flush();
 assert.equal(h.states.at(-1).data.data[0].status,'pending');
 await h.next(); assert.equal(h.states.at(-1).data.data[0].status,'running');
 await h.next(); assert.equal(h.states.at(-1).data.data[0].status,'completed');
 assert.equal(h.completions,1); assert.deepEqual(h.delays,[2000,4000]); assert.equal(h.timers.length,0);
});

test('failed detail remains visible during a different active job outage',async()=>{
 const failed=job('failed',{job_id:'failed',error_code:'artifact_incomplete',recovery_error:'manager_evidence_unavailable'});
 const h=harness({loadPage:async()=>page([failed,job()]),loadJob:async()=>{throw new Error('network');}});await flush();
 for(let i=0;i<5;i++)await h.next();
 const last=h.states.at(-1);
 assert.deepEqual(last.data.data[0],failed); assert.equal(last.data.data[1].status,'pending');
 assert.equal(last.paused,true); assert.equal(h.timers.length,0);
 assert.match(buildFailureDetail(failed.error_code),/remains failed/);
 assert.doesNotMatch(buildFailureDetail('raw private text'),/raw private text/);
});

test('only active jobs are retrieved and failure does not refresh versions',async()=>{
 const ids=[];
 const h=harness({loadPage:async()=>page([job('failed',{job_id:'old'}),job()]),loadJob:async id=>{ids.push(id);return job('failed',{error_code:'worker_exit'});}});await flush();await h.next();
 assert.deepEqual(ids,['job']);assert.equal(h.completions,0);assert.equal(h.timers.length,0);
 assert.equal(h.states.at(-1).data.data[1].error_code,'worker_exit');
});

test('backoff and total rounds are bounded even if a build never finishes',async()=>{
 const h=harness();await flush();
 for(let i=0;i<POLL_MAX_ROUNDS;i++)await h.next();
 assert.deepEqual(h.delays.slice(0,5),[2000,4000,8000,15000,15000]);
 assert.equal(h.requests,POLL_MAX_ROUNDS);assert.equal(h.states.at(-1).paused,true);assert.equal(h.timers.length,0);
});

test('elapsed budget prevents another request after a suspended tab resumes',async()=>{
 const h=harness();await flush();h.advanceTime(POLL_MAX_ELAPSED);await h.next();
 assert.equal(h.requests,0);assert.equal(h.states.at(-1).paused,true);assert.equal(h.timers.length,0);
});

test('transient initial failure recovers; terminal/empty history does not poll',async()=>{
 let attempts=0;
 const h=harness({loadPage:async()=>{if(!attempts++)throw new Error('offline');return page([job('completed')]);}});await flush();
 assert.match(h.states.at(-1).message,/retrying/);await h.next();assert.equal(h.completions,1);assert.equal(h.timers.length,0);
 const empty=harness({loadPage:async()=>page([])});await flush();assert.equal(empty.timers.length,0);
});

test('401/403/404 pause immediately without converting a job to failed',async()=>{
 for(const status of [401,403,404]) {
  const h=harness({loadJob:async()=>{throw {response:{status}};}});await flush();await h.next();
  assert.equal(h.states.at(-1).paused,true);assert.equal(h.states.at(-1).data.data[0].status,'pending');assert.equal(h.timers.length,0);
 }
});

test('wrong site history and wrong job identities never reach the UI',async()=>{
 const foreign=harness({loadPage:async()=>page([job('running',{site_id:'other'})])});await flush();
 assert.equal(foreign.states.at(-1).data,undefined);foreign.stop();
 for(const changed of [{site_id:'other'},{job_id:'other'},{target_version:'other'},{source_version:'other'}]) {
  const h=harness({loadJob:async()=>job('completed',changed)});await flush();await h.next();
  assert.equal(h.states.at(-1).data.data[0].status,'pending');assert.equal(h.completions,0);h.stop();
 }
});

test('stale updates cannot regress progress or revive terminal results',()=>{
 const running=job('running');assert.equal(mergeJob(running,job()),running);
 assert.equal(mergeJob(running,job('completed',{updated_at:'2026-09-06T12:00:00Z'})),running);
 for(const state of ['failed','completed']) {const terminal=job(state);assert.equal(mergeJob(terminal,job('running')),terminal);assert.equal(mergeJob(terminal,job('completed')),terminal);}
});

test('in-flight requests do not overlap; cleanup aborts and suppresses late responses',async()=>{
 let resolve, signal;
 const h=harness({loadJob:(_id,s)=>{signal=s;return new Promise(r=>{resolve=r;});}});await flush();await h.next();
 assert.equal(h.timers.length,0);const count=h.states.length;
 h.stop();assert.equal(signal.aborted,true);resolve(job('completed'));await flush();
 assert.equal(h.states.length,count);assert.equal(h.completions,0);assert.equal(h.timers.length,0);
});

test('cleanup during initial loading and pending timers prevents later work',async()=>{
 let resolve;
 const h=harness({loadPage:()=>new Promise(r=>{resolve=r;})});await flush();h.stop();resolve(page([job('completed')]));await flush();
 assert.equal(h.states.length,0);assert.equal(h.completions,0);
 const ready=harness();await flush();ready.stop();await ready.next();assert.equal(ready.requests,0);
});

test('a successful round resets the consecutive-error budget',async()=>{
 let attempt=0;
 const h=harness({loadJob:async()=>{
  attempt++;
  if(attempt===5)return job('running');
  if(attempt===10)return job('completed');
  throw new Error('temporary outage');
 }});await flush();
 for(let i=0;i<10;i++)await h.next();
 assert.equal(h.states.at(-1).data.data[0].status,'completed');assert.equal(h.completions,1);assert.equal(h.timers.length,0);
});

test('fresh instances recover saved terminal outcomes without polling them',async()=>{
 for(const status of ['failed','completed']) {
  const saved=job(status,{error_code:status==='failed'?'artifact_incomplete':undefined});
  const h=harness({loadPage:async()=>page([saved])});await flush();
  assert.deepEqual(h.states.at(-1).data.data[0],saved);assert.equal(h.requests,0);assert.equal(h.timers.length,0);
 }
});
