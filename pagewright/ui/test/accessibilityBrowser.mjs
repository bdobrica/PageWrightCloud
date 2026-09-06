import assert from 'node:assert/strict';

// Reuses draftBrowser's isolated server/auth/API fixtures. No live services.
export async function auditAccessibility(page, origin, site) {
 const noOverflow = async () => {
  const sizes = await page.evaluate(() => ({width:innerWidth,scroll:document.documentElement.scrollWidth}));
  assert.ok(sizes.scroll <= sizes.width + 1, JSON.stringify(sizes));
 };
 for (const [width,height] of [[1280,900],[768,1024],[390,844],[320,568],[640,450]]) {
  await page.setViewportSize({width,height});
  await page.goto(origin+'/dashboard');
  await page.getByRole('button',{name:'Build',exact:true}).waitFor();
  await noOverflow();
  await page.keyboard.press('Tab');
  assert.equal(await page.evaluate(()=>document.activeElement?.textContent),'Skip to main content');
  await page.keyboard.press('Enter');
  assert.equal(await page.evaluate(()=>document.activeElement?.id),'main-content');
  const controls=await page.locator('.site-card-actions button').evaluateAll(nodes=>nodes.map(node=>({
   height:node.getBoundingClientRect().height, width:node.getBoundingClientRect().width,
  })));
  assert.ok(controls.every(box=>box.height>=44 && box.width>=44));
  await page.screenshot({path:'/tmp/pagewright-m311-'+width+'-dashboard.png',fullPage:true});
  await page.getByRole('button',{name:'Build',exact:true}).focus();
  await page.keyboard.press('Enter');
  await page.waitForURL('**/chat/'+site.fqdn);
  assert.equal(await page.evaluate(()=>document.activeElement?.id),'main-content','route focus missing');
  if (width <= 768) {
   assert.equal(await page.locator('.sidebar details').getAttribute('open'),null);
   await page.screenshot({path:'/tmp/pagewright-m311-'+width+'-collapsed-chat.png',fullPage:true});
   await page.getByText('Browse versions',{exact:true}).focus();
   await page.keyboard.press('Enter');
  }
  const version = page.locator('.version-item').first();
  await version.waitFor();
  await noOverflow();
  const input = page.getByRole('textbox',{name:'Build request'});
  await input.fill('Keyboard draft');
  await input.press('Shift+Enter');
  assert.equal(await input.inputValue(),'Keyboard draft\n');
  await input.dispatchEvent('keydown',{key:'Enter',code:'Enter',isComposing:true,bubbles:true});
  assert.equal(await input.inputValue(),'Keyboard draft\n');
  assert.ok(await page.getByRole('button',{name:'Send',exact:true}).isVisible());
  await page.screenshot({path:'/tmp/pagewright-m311-'+width+'-chat.png',fullPage:true});
  await version.focus();
  await page.keyboard.press('Enter');
  const dialog=page.getByRole('dialog');
  await dialog.waitFor();
  assert.equal(await page.getByRole('button',{name:'Close version actions'}).evaluate(node=>node===document.activeElement),true);
  for(let i=0;i<8;i++) {
   await page.keyboard.press(i<4?'Tab':'Shift+Tab');
   assert.equal(await dialog.evaluate(node=>node.contains(document.activeElement)),true,'modal focus escaped');
  }
  // Background is inert even to a scripted focus attempt while showModal is open.
  await page.getByRole('link',{name:'Dashboard',exact:true}).evaluate(node=>node.focus());
  assert.equal(await dialog.evaluate(node=>node.contains(document.activeElement)),true);
  const box=await dialog.boundingBox();
  assert.ok(box.x>=0 && box.y>=0 && box.x+box.width<=width+1 && box.y+box.height<=height+1,JSON.stringify(box));
  assert.equal(await dialog.evaluate(node=>node.scrollWidth<=node.clientWidth+1),true,'dialog overflows horizontally');
  await noOverflow();
  await page.screenshot({path:'/tmp/pagewright-m311-'+width+'-modal.png'});
  await page.keyboard.press('Escape');
  await dialog.waitFor({state:'detached'});
  assert.equal(await version.evaluate(node=>node===document.activeElement),true,'opener focus not restored');
  assert.equal(await page.evaluate(()=>document.body.style.overflow),'');
  await page.keyboard.press('Space');
  await dialog.waitFor();
  await page.getByRole('button',{name:'Close version actions'}).click();
  assert.equal(await version.evaluate(node=>node===document.activeElement),true);
 }
 // Closing pending work must release focus/scroll without cancelling or reopening it.
 let finish;
 let started;
 const began = new Promise(resolve=>{started=resolve;});
 await page.route('**/versions/*/deploy',route=>{
  started();
  return new Promise(resolve=>{
   finish=async()=>{
    await route.fulfill({status:500,contentType:'application/json',body:'{"message":"fixture outage"}',
     headers:{'Access-Control-Allow-Origin':'*'}});
    resolve();
   };
  });
 });
 const opener=page.locator('.version-item').first();
 await opener.focus();
 await page.keyboard.press('Enter');
 await page.getByRole('button',{name:'Preview in New Tab'}).click();
 await began;
 await page.getByRole('button',{name:'Preparing preview…'}).waitFor();
 await page.getByRole('button',{name:'Close version actions'}).focus();
 await page.keyboard.press('Tab');
 assert.equal(await page.getByRole('dialog').evaluate(node=>node.contains(document.activeElement)),true);
 await page.keyboard.press('Escape');
 assert.equal(await opener.evaluate(node=>node===document.activeElement),true);
 await finish();
 await page.unroute('**/versions/*/deploy');
 assert.equal(await page.getByRole('dialog').count(),0);
 assert.equal(await page.evaluate(()=>document.body.style.overflow),'');
 await page.emulateMedia({reducedMotion:'reduce',colorScheme:'dark'});
 await page.getByRole('button',{name:'Refresh hosting state'}).focus();
 const focus=await page.getByRole('button',{name:'Refresh hosting state'}).evaluate(node=>{
  const style=getComputedStyle(node);
  return {outline:style.outlineStyle,color:style.color,background:style.backgroundColor,transition:style.transitionDuration};
 });
 assert.notEqual(focus.color,focus.background);
 assert.notEqual(focus.outline,'none');
 assert.equal(focus.transition,'0s');
 const contrast = await page.getByRole('button',{name:'Send',exact:true}).evaluate(node=>{
  const style=getComputedStyle(node);
  const luminance=rgb=>{
   const values=rgb.match(/[\d.]+/g).slice(0,3).map(Number).map(value=>{
    const c=value/255; return c<=0.04045?c/12.92:((c+0.055)/1.055)**2.4;
   });
   return values[0]*0.2126+values[1]*0.7152+values[2]*0.0722;
  };
  const fg=luminance(style.color), bg=luminance(style.backgroundColor);
  return (Math.max(fg,bg)+0.05)/(Math.min(fg,bg)+0.05);
 });
 assert.ok(contrast>=4.5,'primary button contrast '+contrast);
 console.log('Keyboard/dialog focus, inert background, Escape/close restoration, IME/newline, touch targets and reflow passed at five viewport sizes, including dark preference and reduced motion.');
}
