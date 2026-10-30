import{$t as e,Bn as t,En as n,Ft as r,Gn as i,Hn as a,In as o,Mt as s,Pt as c,T as l,Yt as u,_n as d,an as f,c as p,cn as m,d as h,en as g,f as _,hn as v,in as y,it as b,jt as x,nn as S,on as C,ot as w,rn as T,rt as E,t as D,tn as O,un as k,wn as A,xn as j,yn as M}from"./api-JTS7hHoA.js";import{n as N,o as P,t as F}from"./Popconfirm-DO5VdxV-.js";import{n as I,r as L,t as R}from"./_plugin-vue_export-helper-CyZKLT9p.js";import{C as z}from"./index-D3QFqE9U.js";function B(t,n){let r=k(w,null);return e(()=>t.hljs||r?.mergedHljsRef.value)}var V=x([s(`code`,`
 font-size: var(--n-font-size);
 font-family: var(--n-font-family);
 `,[r(`show-line-numbers`,`
 display: flex;
 `),c(`line-numbers`,`
 user-select: none;
 padding-right: 12px;
 text-align: right;
 transition: color .3s var(--n-bezier);
 color: var(--n-line-number-text-color);
 `),r(`word-wrap`,[x(`pre`,`
 white-space: pre-wrap;
 word-break: break-all;
 `)]),x(`pre`,`
 margin: 0;
 line-height: inherit;
 font-size: inherit;
 font-family: inherit;
 `),x(`[class^=hljs]`,`
 color: var(--n-text-color);
 transition: 
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 `)]),({props:e})=>{let t=`${e.bPrefix}code`;return[`${t} .hljs-comment,
 ${t} .hljs-quote {
 color: var(--n-mono-3);
 font-style: italic;
 }`,`${t} .hljs-doctag,
 ${t} .hljs-keyword,
 ${t} .hljs-formula {
 color: var(--n-hue-3);
 }`,`${t} .hljs-section,
 ${t} .hljs-name,
 ${t} .hljs-selector-tag,
 ${t} .hljs-deletion,
 ${t} .hljs-subst {
 color: var(--n-hue-5);
 }`,`${t} .hljs-literal {
 color: var(--n-hue-1);
 }`,`${t} .hljs-string,
 ${t} .hljs-regexp,
 ${t} .hljs-addition,
 ${t} .hljs-attribute,
 ${t} .hljs-meta-string {
 color: var(--n-hue-4);
 }`,`${t} .hljs-built_in,
 ${t} .hljs-class .hljs-title {
 color: var(--n-hue-6-2);
 }`,`${t} .hljs-attr,
 ${t} .hljs-variable,
 ${t} .hljs-template-variable,
 ${t} .hljs-type,
 ${t} .hljs-selector-class,
 ${t} .hljs-selector-attr,
 ${t} .hljs-selector-pseudo,
 ${t} .hljs-number {
 color: var(--n-hue-6);
 }`,`${t} .hljs-symbol,
 ${t} .hljs-bullet,
 ${t} .hljs-link,
 ${t} .hljs-meta,
 ${t} .hljs-selector-id,
 ${t} .hljs-title {
 color: var(--n-hue-2);
 }`,`${t} .hljs-emphasis {
 font-style: italic;
 }`,`${t} .hljs-strong {
 font-weight: var(--n-font-weight-strong);
 }`,`${t} .hljs-link {
 text-decoration: underline;
 }`]}]),H=C({name:`Code`,props:Object.assign(Object.assign({},l.props),{language:String,code:{type:String,default:``},trim:{type:Boolean,default:!0},hljs:Object,uri:Boolean,inline:Boolean,wordWrap:Boolean,showLineNumbers:Boolean,internalFontSize:Number,internalNoHighlight:Boolean}),setup(n,{slots:r}){let{internalNoHighlight:i}=n,{mergedClsPrefixRef:a,inlineThemeDisabled:s}=b(),c=o(null),u=i?{value:void 0}:B(n),f=(e,t,n)=>{let{value:r}=u;return!r||!(e&&r.getLanguage(e))?null:r.highlight(n?t.trim():t,{language:e}).value},p=e(()=>n.inline||n.wordWrap?!1:n.showLineNumbers),m=()=>{if(r.default)return;let{value:e}=c;if(!e)return;let{language:t}=n,i=n.uri?window.decodeURIComponent(n.code):n.code;if(t){let r=f(t,i,n.trim);if(r!==null){if(n.inline)e.innerHTML=r;else{let t=e.querySelector(`.__code__`);t&&e.removeChild(t);let n=document.createElement(`pre`);n.className=`__code__`,n.innerHTML=r,e.appendChild(n)}return}}if(n.inline){e.textContent=i;return}let a=e.querySelector(`.__code__`);if(a)a.textContent=i;else{let t=document.createElement(`pre`);t.className=`__code__`,t.textContent=i,e.innerHTML=``,e.appendChild(t)}};d(m),A(t(n,`language`),m),A(t(n,`code`),m),i||A(u,m);let h=l(`Code`,`-code`,V,z,n,a),g=e(()=>{let{common:{cubicBezierEaseInOut:e,fontFamilyMono:t},self:{textColor:r,fontSize:i,fontWeightStrong:a,lineNumberTextColor:o,"mono-3":s,"hue-1":c,"hue-2":l,"hue-3":u,"hue-4":d,"hue-5":f,"hue-5-2":p,"hue-6":m,"hue-6-2":g}}=h.value,{internalFontSize:_}=n;return{"--n-font-size":_?`${_}px`:i,"--n-font-family":t,"--n-font-weight-strong":a,"--n-bezier":e,"--n-text-color":r,"--n-mono-3":s,"--n-hue-1":c,"--n-hue-2":l,"--n-hue-3":u,"--n-hue-4":d,"--n-hue-5":f,"--n-hue-5-2":p,"--n-hue-6":m,"--n-hue-6-2":g,"--n-line-number-text-color":o}}),_=s?E(`code`,e(()=>`${n.internalFontSize||`a`}`),g,n):void 0;return{mergedClsPrefix:a,codeRef:c,mergedShowLineNumbers:p,lineNumbers:e(()=>{let e=1,t=[],r=!1;for(let i of n.code)i===`
`?(r=!0,t.push(e++)):r=!1;return r||t.push(e++),t.join(`
`)}),cssVars:s?void 0:g,themeClass:_?.themeClass,onRender:_?.onRender}},render(){var e;let{mergedClsPrefix:t,wordWrap:n,mergedShowLineNumbers:r,onRender:i}=this;return i?.(),m(`code`,{class:[`${t}-code`,this.themeClass,n&&`${t}-code--word-wrap`,r&&`${t}-code--show-line-numbers`],style:this.cssVars,ref:`codeRef`},r?m(`pre`,{class:`${t}-code__line-numbers`},this.lineNumbers):null,(e=this.$slots).default?.call(e))}}),U={class:`webhook-page`},W={class:`page-header`},G={class:`callback-bar`},K={key:1,class:`message-list`},q=R(C({__name:`WebhookMessagesView`,setup(t){let{t:r,locale:s}=h(),c=L(),l=o([]),m=o(!1),b=null,x=e(()=>`${window.location.origin}/shellink/webhook/callback`);async function C(){m.value=!0;try{l.value=await D(`/webhook-messages`)}catch(e){c.error(e.message)}finally{m.value=!1}}async function w(){try{await D(`/webhook-messages`,{method:`DELETE`}),l.value=[],c.success(r(`webhookMessages.cleared`))}catch(e){c.error(e.message)}}async function E(){try{await navigator.clipboard.writeText(x.value),c.success(r(`webhookMessages.copied`))}catch{c.error(r(`webhookMessages.copyFailed`))}}function k(e){if(!e.data||typeof e.data!=`object`||Array.isArray(e.data))return null;let t=e.data.event;return typeof t==`string`?t:null}function A(e){return JSON.stringify(e,null,2)??String(e)}function R(e){return new Intl.DateTimeFormat(s.value,{dateStyle:`medium`,timeStyle:`medium`}).format(e)}return d(async()=>{await C(),b=new WebSocket(p(`/events`)),b.onmessage=e=>{try{let t=JSON.parse(e.data);if(t.type!==`webhookReceived`)return;l.value=[t,...l.value.filter(e=>e.id!==t.id)].slice(0,200)}catch{}}}),v(()=>b?.close()),(e,t)=>(M(),T(`div`,U,[g(`div`,W,[g(`h1`,null,i(a(r)(`webhookMessages.title`)),1),f(a(I),null,{default:n(()=>[f(a(_),{size:`small`,loading:m.value,onClick:C},{default:n(()=>[y(i(a(r)(`common.refresh`)),1)]),_:1},8,[`loading`]),f(a(F),{disabled:l.value.length===0,onPositiveClick:w},{trigger:n(()=>[f(a(_),{size:`small`,type:`error`,ghost:``,disabled:l.value.length===0},{default:n(()=>[y(i(a(r)(`webhookMessages.clear`)),1)]),_:1},8,[`disabled`])]),default:n(()=>[y(` `+i(a(r)(`webhookMessages.clearConfirm`)),1)]),_:1},8,[`disabled`])]),_:1})]),g(`div`,G,[g(`span`,null,i(a(r)(`webhookMessages.callbackUrl`)),1),g(`code`,null,i(x.value),1),f(a(_),{size:`small`,onClick:E},{default:n(()=>[y(i(a(r)(`dashboard.copy`)),1)]),_:1})]),l.value.length===0&&!m.value?(M(),O(a(P),{key:0,description:a(r)(`webhookMessages.empty`),class:`empty`},null,8,[`description`])):(M(),T(`div`,K,[(M(!0),T(u,null,j(l.value,e=>(M(),T(`article`,{key:e.id,class:`message-item`},[g(`header`,null,[k(e)?(M(),O(a(N),{key:0,size:`small`,type:`info`},{default:n(()=>[y(i(k(e)),1)]),_:2},1024)):S(``,!0),g(`span`,null,i(a(r)(`webhookMessages.receivedAt`))+`: `+i(R(e.receivedAt)),1)]),f(a(H),{code:A(e.data),language:`json`,"word-wrap":``},null,8,[`code`])]))),128))]))]))}}),[[`__scopeId`,`data-v-11a05f62`]]);export{q as default};