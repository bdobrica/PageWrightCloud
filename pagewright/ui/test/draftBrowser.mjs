// Focused rendered regression; mocked gateway, no provider or real user data.
// Build first. Pass installed Playwright module and Firefox executable paths.
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { resolve, extname } from 'node:path';
import { auditAccessibility } from './accessibilityBrowser.mjs';
import { auditReset } from './resetBrowser.mjs';

const [modulePath, executablePath] = process.argv.slice(2);
if (!modulePath || !executablePath) throw Error('Pass Playwright module and Firefox executable paths');
const { firefox } = await import(pathToFileURL(modulePath).href);
const root = fileURLToPath(new URL('../dist/', import.meta.url));
const server = createServer(async (req,res) => {
 try {
  let file = resolve(root, '.' + new URL(req.url,'http://localhost').pathname);
  if (!file.startsWith(root)) { res.writeHead(403).end(); return; }
  if (!new URL(req.url,'http://localhost').pathname.startsWith('/assets/')) file = resolve(root,'index.html');
  const data = await readFile(file);
  res.setHeader('Content-Type', extname(file)==='.js'?'application/javascript':extname(file)==='.css'?'text/css':'text/html');
  res.end(data);
 } catch { res.writeHead(404).end(); }
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
let browser;
try {
 browser = await firefox.launch({headless:true,executablePath});
 const page = await browser.newPage({viewport:null});
 page.setDefaultTimeout(10000);
 const origin = 'http://127.0.0.1:'+server.address().port;
 const user={id:'owner-a',email:'a@example.test',created_at:'2026-09-06T12:00:00Z'};
 const site={id:'site-a',fqdn:'one.example.test',user_id:user.id,enabled:true,
  initialization_status:'ready',template_id:'starter',created_at:user.created_at,updated_at:user.created_at,
  live_url:'http://one.example.test:8084/',preview_url:'http://one.preview.example.test:8084/'};
 const version={id:'v1',site_id:site.id,build_id:'v1',status:'completed',created_at:user.created_at};
 const accessibility = process.argv.includes('--accessibility');
 if (accessibility) {
  site.fqdn='a'.repeat(55)+'.example.test';
  site.live_url='http://'+site.fqdn+':8084/';
  site.preview_url='http://'+site.fqdn.replace('.', '.preview.')+':8084/';
  version.build_id='v1-'+'b'.repeat(80);
  version.id=version.build_id;
  site.live_version_id=version.build_id;
 }
 let buildMode='expired', publishFails=true, versionsFail=false, toggleFails=false;
 const submissions=[];
 const errors=[];
 page.on('pageerror',error=>errors.push(error.message));
 page.on('dialog',dialog=>dialog.accept());
 const paginate=(data,size=25)=>({data,page:1,page_size:size,total_count:data.length,total_pages:data.length?1:0});
 await page.route('http://localhost:8085/**',async route=>{
  const req=route.request(), url=new URL(req.url()), path=url.pathname;
  let status=200, body={};
  if(req.method()==='OPTIONS') body={};
  else if(path==='/auth/login') {
   if(req.postDataJSON().password==='wrong') {status=401;body={message:'Invalid credentials'};}
   else body={token:'new-token',user,expires_in:900};
  } else if(path.endsWith('/build')) {
   assert.equal(req.headers()['x-pagewright-draft-owner'],undefined,'local owner guard must not leave the browser');
   submissions.push({key:req.headers()['idempotency-key'],body:req.postDataJSON()});
   if(buildMode==='expired'){status=401;body={message:'Session expired'};}
   else if(buildMode==='uncertain'){status=503;body={message:'Confirmation unavailable'};}
   else if(buildMode==='question') body={question:'Which title?',conversation_id:'conversation-a'};
   else body={job_id:'job-a',site_id:site.id,owner_id:user.id,source_version:'initial',target_version:'v1',status:'completed'};
  } else if(path.endsWith('/deploy')) {
   if(publishFails){status=500;body={message:'Unavailable'};}
   else {site.live_version_id='v1';body={status:'deployed',version_id:'v1',target:'live',url:site.live_url};}
  } else if(path.endsWith('/disable')) {site.enabled=false;}
  else if(path.endsWith('/enable')) {
   if(toggleFails){status=503;body={message:'Toggle unavailable'};} else site.enabled=true;
  }
  else if(path.endsWith('/versions')) {
   if(versionsFail){status=503;} else body=paginate([version],10);
  } else if(path.endsWith('/jobs')) body=paginate([]);
  else if(path==='/sites') body=paginate([site]);
  else if(path==='/sites/'+site.fqdn) body=site;
  else {status=404;}
  await route.fulfill({status,contentType:'application/json',body:JSON.stringify(body),
   headers:{'Access-Control-Allow-Origin':'*','Access-Control-Allow-Headers':'Authorization,Content-Type,Idempotency-Key',
    'Access-Control-Allow-Methods':'GET,POST,OPTIONS'}});
 });
 await page.goto(origin+'/login');
 await page.evaluate(user=>{
  localStorage.setItem('user',JSON.stringify(user));localStorage.setItem('token','old-token');
 },user);
 await page.goto(origin+'/chat/'+site.fqdn);
 if (!accessibility) {
  const urls = {live_url:site.live_url, preview_url:site.preview_url};
  site.hosting_status='provisioning';site.live_url='';site.preview_url='';
  await page.reload();
  await page.getByText('HTTPS provisioning — refresh shortly.',{exact:true}).waitFor();
  assert.equal(await page.getByRole('button',{name:'View Live',exact:true}).isDisabled(),true);
  assert.equal(await page.getByRole('button',{name:'View Preview',exact:true}).isDisabled(),true);
  delete site.hosting_status;Object.assign(site,urls);
  await page.reload();
 }
 // The recovery test is desktop; the accessibility branch exercises collapsed mobile versions.
 if (!accessibility && !await page.locator('.sidebar details').evaluate(node=>node.open)) {
  await page.getByText('Browse versions',{exact:true}).click();
 }
 if (accessibility) {
  await auditAccessibility(page,origin,site);
  assert.equal(submissions.length,0,'IME/newline must not submit');
  assert.deepEqual(errors,[]);
 } else {
 const input=page.getByRole('textbox',{name:'Build request'});
 await input.fill('Keep my unsent edit');
 await page.reload();
 assert.equal(await input.inputValue(),'Keep my unsent edit');
 await page.getByRole('button',{name:'Send',exact:true}).click();
 await page.waitForURL('**/login');
 await page.getByLabel('Email').fill(user.email);
 await page.getByLabel('Password',{exact:true}).fill('wrong');
 await page.getByRole('button',{name:'Login',exact:true}).click();
 await page.getByText('Invalid credentials',{exact:true}).waitFor();
 assert.equal(await page.getByLabel('Email').inputValue(),user.email);
 buildMode='uncertain';
 await page.getByLabel('Password',{exact:true}).fill('correct');
 await page.getByRole('button',{name:'Login',exact:true}).click();
 await page.waitForURL('**/chat/'+site.fqdn);
 assert.equal(await input.inputValue(),'Keep my unsent edit');
 assert.equal(submissions.length,1,'login must not auto-submit');
 await page.getByRole('button',{name:'Retry same request'}).click();
 await page.getByRole('alert').filter({hasText:'Confirmation unavailable'}).waitFor();
 await page.reload();
 assert.equal(await input.inputValue(),'Keep my unsent edit');
 buildMode='accepted';
 await page.getByRole('button',{name:'Retry same request'}).click();
 await page.waitForFunction(()=>document.querySelector('textarea')?.value==='');
 assert.equal(submissions.length,3);
 assert.ok(submissions.every(s=>s.key===submissions[0].key));
 assert.deepEqual(submissions[0].body,submissions[2].body);

 await input.fill('Account A private draft');
 await page.evaluate(user=>localStorage.setItem('user',JSON.stringify({...user,id:'owner-b'})),user);
 await page.reload();
 assert.equal(await input.inputValue(),'');
 await page.evaluate(user=>localStorage.setItem('user',JSON.stringify(user)),user);
 await page.reload();
 assert.equal(await input.inputValue(),'Account A private draft');
 buildMode='question';
 await page.getByRole('button',{name:'Send',exact:true}).click();
 await page.getByText('Clarification: Which title?',{exact:true}).waitFor();
 await input.fill('A clearer title');
 await page.reload();
 assert.equal(await input.inputValue(),'A clearer title');
 await page.getByText('Original request: Account A private draft',{exact:true}).waitFor();
 await page.getByText('Clarification: Which title?',{exact:true}).waitFor();

 await page.getByRole('button',{name:/Saved build.*v1/}).click();
 await page.getByRole('button',{name:'Promote to Live',exact:true}).click();
 await page.getByRole('alert').filter({hasText:'Publishing could not be confirmed'}).waitFor();
 publishFails=false;
 await page.getByRole('button',{name:'Promote to Live',exact:true}).click();
 await page.getByRole('button',{name:/Live.*v1/}).waitFor();
 await page.goto(origin+'/dashboard');
 await page.getByText('Live Version:',{exact:true}).waitFor();
 assert.match(await page.locator('.site-card-info').innerText(),/Live Version: v1/);
 await page.getByRole('button',{name:'Disable',exact:true}).click();
 await page.getByRole('button',{name:'Enable',exact:true}).waitFor();
 site.live_version_id='v2';
 await page.evaluate(()=>window.dispatchEvent(new Event('focus')));
 await page.locator('.site-card-info').filter({hasText:'Live Version: v2'}).waitFor();
 toggleFails=true;
 await page.getByRole('button',{name:'Enable',exact:true}).click();
 await page.getByRole('alert').filter({hasText:'Toggle unavailable'}).waitFor();
 await page.getByRole('button',{name:'Enable',exact:true}).waitFor();
 assert.equal(site.enabled,false);
 toggleFails=false;
 await page.getByRole('button',{name:'Enable',exact:true}).click();
 await page.getByRole('button',{name:'Disable',exact:true}).waitFor();
 site.live_version_id='v1';

 versionsFail=true;
 await page.goto(origin+'/chat/'+site.fqdn);
 await page.getByText('Unable to load versions. Use Refresh versions to retry.',{exact:true}).waitFor();
 versionsFail=false;
 await page.getByRole('button',{name:'Refresh versions',exact:true}).click();
 await page.getByRole('button',{name:/Live.*v1/}).waitFor();
 const count=submissions.length;
 await page.evaluate(()=>{
  const original=Storage.prototype.setItem;
  Storage.prototype.setItem=function(key,value) {
   if(key.startsWith('pagewright.draft.')) throw Error('simulated quota');
   return original.call(this,key,value);
  };
 });
 await input.fill('Unsaved text must not submit');
 await page.getByRole('button',{name:'Send',exact:true}).click();
 await page.getByRole('alert').filter({hasText:'Draft could not be saved'}).waitFor();
 assert.equal(submissions.length,count);
 assert.equal(await input.inputValue(),'Unsaved text must not submit');
 await page.reload(); // Restore the normal storage implementation.
 const storageKey='pagewright.draft.v1:'+JSON.stringify([user.id,site.fqdn]);
 await page.evaluate(key=>sessionStorage.setItem(key,'broken'),storageKey);
 await page.reload();
 await page.getByRole('button',{name:'Retry loading saved draft'}).waitFor();
 assert.equal(await input.isDisabled(),true);
 assert.equal(await page.evaluate(key=>sessionStorage.getItem(key),storageKey),'broken');
 await page.evaluate(key=>sessionStorage.setItem(key,JSON.stringify({text:'Recovered after storage repair'})),storageKey);
 await page.getByRole('button',{name:'Retry loading saved draft'}).click();
 assert.equal(await input.inputValue(),'Recovered after storage repair');
 assert.equal(submissions.length,count);
 await page.getByRole('button',{name:'Logout',exact:true}).click();
 await page.waitForURL('**/login');
 assert.equal(await page.evaluate(key=>sessionStorage.getItem(key),storageKey),null);
 assert.equal(await page.evaluate(()=>localStorage.getItem('token')),null);
 assert.deepEqual(errors,[]);
 console.log('Rendered draft expiry/re-auth/retry, owner isolation, clarification, publishing, dashboard refresh, version retry and storage-failure checks passed.');
 await auditReset(browser, origin);
 }
} finally {
 if(browser) await browser.close();
 await new Promise(resolve=>server.close(resolve));
}
