import{$t as e,En as t,Ft as n,Gn as r,Hn as i,Ht as a,In as o,Lt as s,Mt as c,T as l,Tn as u,_n as d,an as f,b as p,cn as m,d as h,en as g,f as _,in as v,it as y,jt as b,kt as x,on as S,rn as C,rt as w,t as T,v as E,y as D,yn as O}from"./api-JTS7hHoA.js";import{t as k}from"./use-compitable-rVRlQ9mX.js";import{I as A,L as j,l as M}from"./index-D3QFqE9U.js";var N=b([b(`@keyframes spin-rotate`,`
 from {
 transform: rotate(0);
 }
 to {
 transform: rotate(360deg);
 }
 `),c(`spin-container`,`
 position: relative;
 `,[c(`spin-body`,`
 position: absolute;
 top: 50%;
 left: 50%;
 transform: translateX(-50%) translateY(-50%);
 `,[E()])]),c(`spin-body`,`
 display: inline-flex;
 align-items: center;
 justify-content: center;
 flex-direction: column;
 `),c(`spin`,`
 display: inline-flex;
 height: var(--n-size);
 width: var(--n-size);
 font-size: var(--n-size);
 color: var(--n-color);
 `,[n(`rotate`,`
 animation: spin-rotate 2s linear infinite;
 `)]),c(`spin-description`,`
 display: inline-block;
 font-size: var(--n-font-size);
 color: var(--n-text-color);
 transition: color .3s var(--n-bezier);
 margin-top: 8px;
 `),c(`spin-content`,`
 opacity: 1;
 transition: opacity .3s var(--n-bezier);
 pointer-events: all;
 `,[n(`spinning`,`
 user-select: none;
 -webkit-user-select: none;
 pointer-events: none;
 opacity: var(--n-opacity-spinning);
 `)])]),P={small:20,medium:18,large:16},F=S({name:`Spin`,props:Object.assign(Object.assign(Object.assign({},l.props),{contentClass:String,contentStyle:[Object,String],description:String,size:{type:[String,Number],default:`medium`},show:{type:Boolean,default:!0},rotate:{type:Boolean,default:!0},spinning:{type:Boolean,validator:()=>!0,default:void 0},delay:Number}),p),slots:Object,setup(t){let{mergedClsPrefixRef:n,inlineThemeDisabled:r}=y(t),i=l(`Spin`,`-spin`,N,M,t,n),a=e(()=>{let{size:e}=t,{common:{cubicBezierEaseInOut:n},self:r}=i.value,{opacitySpinning:a,color:o,textColor:c}=r;return{"--n-bezier":n,"--n-opacity-spinning":a,"--n-size":typeof e==`number`?x(e):r[s(`size`,e)],"--n-color":o,"--n-text-color":c}}),c=r?w(`spin`,e(()=>{let{size:e}=t;return typeof e==`number`?String(e):e[0]}),a,t):void 0,d=k(t,[`spinning`,`show`]),f=o(!1);return u(e=>{let n;if(d.value){let{delay:r}=t;if(r){n=window.setTimeout(()=>{f.value=!0},r),e(()=>{clearTimeout(n)});return}}f.value=d.value}),{mergedClsPrefix:n,active:f,mergedStrokeWidth:e(()=>{let{strokeWidth:e}=t;if(e!==void 0)return e;let{size:n}=t;return P[typeof n==`number`?`medium`:n]}),cssVars:r?void 0:a,themeClass:c?.themeClass,onRender:c?.onRender}},render(){var e;let{$slots:t,mergedClsPrefix:n,description:r}=this,i=t.icon&&this.rotate,o=(r||t.description)&&m(`div`,{class:`${n}-spin-description`},r||t.description?.call(t)),s=t.icon?m(`div`,{class:[`${n}-spin-body`,this.themeClass]},m(`div`,{class:[`${n}-spin`,i&&`${n}-spin--rotate`],style:t.default?``:this.cssVars},t.icon()),o):m(`div`,{class:[`${n}-spin-body`,this.themeClass]},m(D,{clsPrefix:n,style:t.default?``:this.cssVars,stroke:this.stroke,"stroke-width":this.mergedStrokeWidth,radius:this.radius,scale:this.scale,class:`${n}-spin`}),o);return(e=this.onRender)==null||e.call(this),t.default?m(`div`,{class:[`${n}-spin-container`,this.themeClass],style:this.cssVars},m(`div`,{class:[`${n}-spin-content`,this.active&&`${n}-spin-content--spinning`,this.contentClass],style:this.contentStyle},t),m(a,{name:`fade-in-transition`},{default:()=>this.active?s:null})):s}}),I={class:`page-header`},L={style:{display:`flex`,"align-items":`center`,gap:`12px`}},R={class:`history-block`},z=S({__name:`HistoryView`,setup(e){let n=A(),a=j(),s=n.params.id,{t:c}=h(),l=o(``),u=o(!1),p=0;async function m(e=!1){u.value=!0;try{for(e&&(p=0,l.value=``);;){let e=await T(`/sessions/${s}/history?since=${p}&limit=5000`);if(l.value+=e.text,e.cursor===p)break;p=e.cursor}}finally{u.value=!1}}return d(()=>m()),(e,n)=>(O(),C(`div`,null,[g(`div`,I,[g(`div`,L,[f(i(_),{size:`small`,quaternary:``,onClick:n[0]||=e=>i(a).back()},{default:t(()=>[v(` ← `+r(i(c)(`common.back`)),1)]),_:1}),g(`h1`,null,r(i(c)(`history.title`))+` · `+r(i(s)),1)]),f(i(_),{size:`small`,loading:u.value,onClick:n[1]||=e=>m(!0)},{default:t(()=>[v(r(i(c)(`common.refresh`)),1)]),_:1},8,[`loading`])]),f(i(F),{show:u.value},{default:t(()=>[g(`pre`,R,r(l.value||i(c)(`history.empty`)),1)]),_:1},8,[`show`])]))}});export{z as default};