import{$t as e,Bn as t,Dn as n,Et as r,Ft as i,Gt as a,Ht as o,In as s,It as c,Lt as l,Mt as u,O as d,Ot as f,Pt as p,Sn as m,T as h,Tn as g,Yt as _,_n as v,_t as y,bn as b,bt as x,cn as S,dn as C,dt as w,fn as T,ft as E,gn as D,hn as O,it as ee,jt as k,kt as A,lt as j,nt as te,on as M,pn as N,rt as P,un as F,wn as I,xt as L,y as ne}from"./api-JTS7hHoA.js";import{d as R,f as z,g as re,h as B,l as ie,m as ae,n as V,o as oe,p as H,r as U,u as W,v as G}from"./Popconfirm-DO5VdxV-.js";import{C as K,E as se,S as q,a as J,b as Y,u as ce}from"./light-55e3aIrt.js";import{y as X}from"./light-xVEcws8X.js";import{i as le,o as ue}from"./_plugin-vue_export-helper-CyZKLT9p.js";import{t as de}from"./use-compitable-rVRlQ9mX.js";import{t as fe}from"./Suffix-ByC5Skhl.js";import{A as pe,F as me,O as he,P as Z,k as ge,x as _e}from"./index-D3QFqE9U.js";function Q(e,t){let{target:n}=e;for(;n;){if(n.dataset&&n.dataset[t]!==void 0)return!0;n=n.parentElement}return!1}function ve(e){return e&-e}var ye=class{constructor(e,t){this.l=e,this.min=t;let n=Array(e+1);for(let t=0;t<e+1;++t)n[t]=0;this.ft=n}add(e,t){if(t===0)return;let{l:n,ft:r}=this;for(e+=1;e<=n;)r[e]+=t,e+=ve(e)}get(e){return this.sum(e+1)-this.sum(e)}sum(e){if(e===void 0&&(e=this.l),e<=0)return 0;let{ft:t,min:n,l:r}=this;if(e>r)throw Error("[FinweckTree.sum]: `i` is larger than length.");let i=e*n;for(;e>0;)i+=t[e],e-=ve(e);return i}getBound(e){let t=0,n=this.l;for(;n>t;){let r=Math.floor((t+n)/2),i=this.sum(r);if(i>e){n=r;continue}else if(i<e){if(t===r)return this.sum(t+1)<=e?t+1:r;t=r}else return r}return t}},be;function xe(){return typeof document>`u`?!1:(be===void 0&&(be=`matchMedia`in window&&window.matchMedia(`(pointer:coarse)`).matches),be)}var Se;function Ce(){return typeof document>`u`?1:(Se===void 0&&(Se=`chrome`in window?window.devicePixelRatio:1),Se)}var we=`VVirtualListXScroll`;function Te({columnsRef:t,renderColRef:n,renderItemWithColsRef:r}){let i=s(0),a=s(0),o=e(()=>{let e=t.value;if(e.length===0)return null;let n=new ye(e.length,0);return e.forEach((e,t)=>{n.add(t,e.width)}),n});return b(we,{startIndexRef:L(()=>{let e=o.value;return e===null?0:Math.max(e.getBound(a.value)-1,0)}),endIndexRef:L(()=>{let e=o.value;return e===null?0:Math.min(e.getBound(a.value+i.value)+1,t.value.length-1)}),columnsRef:t,renderColRef:n,renderItemWithColsRef:r,getLeft:e=>{let t=o.value;return t===null?0:t.sum(e)}}),{listWidthRef:i,scrollLeftRef:a}}var Ee=M({name:`VirtualListRow`,props:{index:{type:Number,required:!0},item:{type:Object,required:!0}},setup(){let{startIndexRef:e,endIndexRef:t,columnsRef:n,getLeft:r,renderColRef:i,renderItemWithColsRef:a}=F(we);return{startIndex:e,endIndex:t,columns:n,renderCol:i,renderItemWithCols:a,getLeft:r}},render(){let{startIndex:e,endIndex:t,columns:n,renderCol:r,renderItemWithCols:i,getLeft:a,item:o}=this;if(i!=null)return i({itemIndex:this.index,startColIndex:e,endColIndex:t,allColumns:n,item:o,getLeft:a});if(r!=null){let i=[];for(let s=e;s<=t;++s){let e=n[s];i.push(r({column:e,left:a(s),item:o}))}return i}return null}}),De=W(`.v-vl`,{maxHeight:`inherit`,height:`100%`,overflow:`auto`,minWidth:`1px`},[W(`&:not(.v-vl--show-scrollbar)`,{scrollbarWidth:`none`},[W(`&::-webkit-scrollbar, &::-webkit-scrollbar-track-piece, &::-webkit-scrollbar-thumb`,{width:0,height:0,display:`none`})])]),Oe=M({name:`VirtualList`,inheritAttrs:!1,props:{showScrollbar:{type:Boolean,default:!0},columns:{type:Array,default:()=>[]},renderCol:Function,renderItemWithCols:Function,items:{type:Array,default:()=>[]},itemSize:{type:Number,required:!0},itemResizable:Boolean,itemsStyle:[String,Object],visibleItemsTag:{type:[String,Object],default:`div`},visibleItemsProps:Object,ignoreItemResize:Boolean,onScroll:Function,onWheel:Function,onResize:Function,defaultScrollKey:[Number,String],defaultScrollIndex:Number,keyField:{type:String,default:`key`},paddingTop:{type:[Number,String],default:0},paddingBottom:{type:[Number,String],default:0}},setup(n){let i=y();De.mount({id:`vueuc/virtual-list`,head:!0,anchorMetaName:R,ssr:i}),v(()=>{let{defaultScrollIndex:e,defaultScrollKey:t}=n;e==null?t!=null&&S({key:t}):S({index:e})});let a=!1,o=!1;N(()=>{if(a=!1,!o){o=!0;return}S({top:_.value,left:u.value})}),D(()=>{a=!0,o||=!0});let c=L(()=>{if(n.renderCol==null&&n.renderItemWithCols==null||n.columns.length===0)return;let e=0;return n.columns.forEach(t=>{e+=t.width}),e}),l=e(()=>{let e=new Map,{keyField:t}=n;return n.items.forEach((n,r)=>{e.set(n[t],r)}),e}),{scrollLeftRef:u,listWidthRef:d}=Te({columnsRef:t(n,`columns`),renderColRef:t(n,`renderCol`),renderItemWithColsRef:t(n,`renderItemWithCols`)}),f=s(null),p=s(void 0),m=new Map,h=e(()=>{let{items:e,itemSize:t,keyField:r}=n,i=new ye(e.length,t);return e.forEach((e,t)=>{let n=e[r],a=m.get(n);a!==void 0&&i.add(t,a)}),i}),g=s(0),_=s(0),b=L(()=>Math.max(h.value.getBound(_.value-r(n.paddingTop))-1,0)),x=e(()=>{let{value:e}=p;if(e===void 0)return[];let{items:t,itemSize:r}=n,i=b.value,a=Math.min(i+Math.ceil(e/r+1),t.length-1),o=[];for(let e=i;e<=a;++e)o.push(t[e]);return o}),S=(e,t)=>{if(typeof e==`number`){E(e,t,`auto`);return}let{left:n,top:r,index:i,key:a,position:o,behavior:s,debounce:c=!0}=e;if(n!==void 0||r!==void 0)E(n,r,s);else if(i!==void 0)T(i,s,c);else if(a!==void 0){let e=l.value.get(a);e!==void 0&&T(e,s,c)}else o===`bottom`?E(0,2**53-1,s):o===`top`&&E(0,0,s)},C,w=null;function T(e,t,i){let{value:a}=h,o=a.sum(e)+r(n.paddingTop);if(!i)f.value.scrollTo({left:0,top:o,behavior:t});else{C=e,w!==null&&window.clearTimeout(w),w=window.setTimeout(()=>{C=void 0,w=null},16);let{scrollTop:n,offsetHeight:r}=f.value;if(o>n){let i=a.get(e);o+i<=n+r||f.value.scrollTo({left:0,top:o+i-r,behavior:t})}else f.value.scrollTo({left:0,top:o,behavior:t})}}function E(e,t,n){f.value.scrollTo({left:e,top:t,behavior:n})}function O(e,t){if(a||n.ignoreItemResize||F(t.target))return;let{value:r}=h,i=l.value.get(e),o=r.get(i),s=t.borderBoxSize?.[0]?.blockSize??t.contentRect.height;if(s===o)return;s-n.itemSize===0?m.delete(e):m.set(e,s-n.itemSize);let c=s-o;if(c===0)return;r.add(i,c);let u=f.value;if(u!=null){if(C===void 0){let e=r.sum(i);u.scrollTop>e&&u.scrollBy(0,c)}else(i<C||i===C&&s+r.sum(i)>u.scrollTop+u.offsetHeight)&&u.scrollBy(0,c);P()}g.value++}let ee=!xe(),k=!1;function j(e){var t;(t=n.onScroll)==null||t.call(n,e),(!ee||!k)&&P()}function te(e){var t;if((t=n.onWheel)==null||t.call(n,e),ee){let t=f.value;if(t!=null){if(e.deltaX===0&&(t.scrollTop===0&&e.deltaY<=0||t.scrollTop+t.offsetHeight>=t.scrollHeight&&e.deltaY>=0))return;e.preventDefault(),t.scrollTop+=e.deltaY/Ce(),t.scrollLeft+=e.deltaX/Ce(),P(),k=!0,G(()=>{k=!1})}}}function M(e){if(a||F(e.target))return;if(n.renderCol==null&&n.renderItemWithCols==null){if(e.contentRect.height===p.value)return}else if(e.contentRect.height===p.value&&e.contentRect.width===d.value)return;p.value=e.contentRect.height,d.value=e.contentRect.width;let{onResize:t}=n;t!==void 0&&t(e)}function P(){let{value:e}=f;e!=null&&(_.value=e.scrollTop,u.value=e.scrollLeft)}function F(e){let t=e;for(;t!==null;){if(t.style.display===`none`)return!0;t=t.parentElement}return!1}return{listHeight:p,listStyle:{overflow:`auto`},keyToIndex:l,itemsStyle:e(()=>{let{itemResizable:e}=n,t=A(h.value.sum());return g.value,[n.itemsStyle,{boxSizing:`content-box`,width:A(c.value),height:e?``:t,minHeight:e?t:``,paddingTop:A(n.paddingTop),paddingBottom:A(n.paddingBottom)}]}),visibleItemsStyle:e(()=>(g.value,{transform:`translateY(${A(h.value.sum(b.value))})`})),viewportItems:x,listElRef:f,itemsElRef:s(null),scrollTo:S,handleListResize:M,handleListScroll:j,handleListWheel:te,handleItemResize:O}},render(){let{itemResizable:e,keyField:t,keyToIndex:n,visibleItemsTag:r}=this;return S(q,{onResize:this.handleListResize},{default:()=>{var i;return S(`div`,C(this.$attrs,{class:[`v-vl`,this.showScrollbar&&`v-vl--show-scrollbar`],onScroll:this.handleListScroll,onWheel:this.handleListWheel,ref:`listElRef`}),[this.items.length===0?(i=this.$slots).empty?.call(i):S(`div`,{ref:`itemsElRef`,class:`v-vl-items`,style:this.itemsStyle},[S(r,Object.assign({class:`v-vl-visible-items`,style:this.visibleItemsStyle},this.visibleItemsProps),{default:()=>{let{renderCol:r,renderItemWithCols:i}=this;return this.viewportItems.map(a=>{let o=a[t],s=n.get(o),c=r==null?void 0:S(Ee,{index:s,item:a}),l=i==null?void 0:S(Ee,{index:s,item:a}),u=this.$slots.default({item:a,renderedCols:c,renderedItemWithCols:l,index:s})[0];return e?S(q,{key:o,onResize:e=>this.handleItemResize(o,e)},{default:()=>u}):(u.key=o,u)})}})])])}})}}),$=`v-hidden`,ke=W(`[v-hidden]`,{display:`none!important`}),Ae=M({name:`Overflow`,props:{getCounter:Function,getTail:Function,updateCounter:Function,onUpdateCount:Function,onUpdateOverflow:Function},setup(e,{slots:t}){let n=s(null),r=s(null);function i(i){let{value:a}=n,{getCounter:o,getTail:s}=e,c;if(c=o===void 0?r.value:o(),!a||!c)return;c.hasAttribute($)&&c.removeAttribute($);let{children:l}=a;if(i.showAllItemsBeforeCalculate)for(let e of l)e.hasAttribute($)&&e.removeAttribute($);let u=a.offsetWidth,d=[],f=t.tail?s?.():null,p=f?f.offsetWidth:0,m=!1,h=a.children.length-+!!t.tail;for(let t=0;t<h-1;++t){if(t<0)continue;let n=l[t];if(m){n.hasAttribute($)||n.setAttribute($,``);continue}else n.hasAttribute($)&&n.removeAttribute($);let r=n.offsetWidth;if(p+=r,d[t]=r,p>u){let{updateCounter:n}=e;for(let r=t;r>=0;--r){let i=h-1-r;n===void 0?c.textContent=`${i}`:n(i);let a=c.offsetWidth;if(p-=d[r],p+a<=u||r===0){m=!0,t=r-1,f&&(t===-1?(f.style.maxWidth=`${u-a}px`,f.style.boxSizing=`border-box`):f.style.maxWidth=``);let{onUpdateCount:n}=e;n&&n(i);break}}}}let{onUpdateOverflow:g}=e;m?g!==void 0&&g(!0):(g!==void 0&&g(!1),c.setAttribute($,``))}let a=y();return ke.mount({id:`vueuc/overflow`,head:!0,anchorMetaName:R,ssr:a}),v(()=>i({showAllItemsBeforeCalculate:!1})),{selfRef:n,counterRef:r,sync:i}},render(){let{$slots:e}=this;return T(()=>this.sync({showAllItemsBeforeCalculate:!1})),S(`div`,{class:`v-overflow`,ref:`selfRef`},[m(e,`default`),e.counter?e.counter():S(`span`,{style:{display:`inline-block`},ref:`counterRef`}),e.tail?e.tail():null])}});function je(e,t){t&&(v(()=>{let{value:n}=e;n&&K.registerHandler(n,t)}),I(e,(e,t)=>{t&&K.unregisterHandler(t)},{deep:!1}),O(()=>{let{value:t}=e;t&&K.unregisterHandler(t)}))}function Me(e){switch(typeof e){case`string`:return e||void 0;case`number`:return String(e);default:return}}function Ne(e){let t=e.filter(e=>e!==void 0);if(t.length!==0)return t.length===1?t[0]:t=>{e.forEach(e=>{e&&e(t)})}}var Pe=M({name:`Checkmark`,render(){return S(`svg`,{xmlns:`http://www.w3.org/2000/svg`,viewBox:`0 0 16 16`},S(`g`,{fill:`none`},S(`path`,{d:`M14.046 3.486a.75.75 0 0 1-.032 1.06l-7.93 7.474a.85.85 0 0 1-1.188-.022l-2.68-2.72a.75.75 0 1 1 1.068-1.053l2.234 2.267l7.468-7.038a.75.75 0 0 1 1.06.032z`,fill:`currentColor`})))}}),Fe=M({props:{onFocus:Function,onBlur:Function},setup(e){return()=>S(`div`,{style:`width: 0; height: 0`,tabindex:0,onFocus:e.onFocus,onBlur:e.onBlur})}});function Ie(e){return Array.isArray(e)?e:[e]}var Le={STOP:`STOP`};function Re(e,t){let n=t(e);e.children!==void 0&&n!==Le.STOP&&e.children.forEach(e=>Re(e,t))}function ze(e,t={}){let{preserveGroup:n=!1}=t,r=[],i=n?e=>{e.isLeaf||(r.push(e.key),a(e.children))}:e=>{e.isLeaf||(e.isGroup||r.push(e.key),a(e.children))};function a(e){e.forEach(i)}return a(e),r}function Be(e,t){let{isLeaf:n}=e;return n===void 0?!t(e):n}function Ve(e){return e.children}function He(e){return e.key}function Ue(){return!1}function We(e,t){let{isLeaf:n}=e;return!(n===!1&&!Array.isArray(t(e)))}function Ge(e){return e.disabled===!0}function Ke(e,t){return e.isLeaf===!1&&!Array.isArray(t(e))}function qe(e){return e==null?[]:Array.isArray(e)?e:e.checkedKeys??[]}function Je(e){return e==null||Array.isArray(e)?[]:e.indeterminateKeys??[]}function Ye(e,t){let n=new Set(e);return t.forEach(e=>{n.has(e)||n.add(e)}),Array.from(n)}function Xe(e,t){let n=new Set(e);return t.forEach(e=>{n.has(e)&&n.delete(e)}),Array.from(n)}function Ze(e){return e?.type===`group`}function Qe(e){let t=new Map;return e.forEach((e,n)=>{t.set(e.key,n)}),e=>t.get(e)??null}var $e=class extends Error{constructor(){super(),this.message=`SubtreeNotLoadedError: checking a subtree whose required nodes are not fully loaded.`}};function et(e,t,n,r){return it(t.concat(e),n,r,!1)}function tt(e,t){let n=new Set;return e.forEach(e=>{let r=t.treeNodeMap.get(e);if(r!==void 0){let e=r.parent;for(;e!==null&&!(e.disabled||n.has(e.key));)n.add(e.key),e=e.parent}}),n}function nt(e,t,n,r){let i=it(t,n,r,!1),a=it(e,n,r,!0),o=tt(e,n),s=[];return i.forEach(e=>{(a.has(e)||o.has(e))&&s.push(e)}),s.forEach(e=>i.delete(e)),i}function rt(e,t){let{checkedKeys:n,keysToCheck:r,keysToUncheck:i,indeterminateKeys:a,cascade:o,leafOnly:s,checkStrategy:c,allowNotLoaded:l}=e;if(!o)return r===void 0?i===void 0?{checkedKeys:Array.from(n),indeterminateKeys:Array.from(a)}:{checkedKeys:Xe(n,i),indeterminateKeys:Array.from(a)}:{checkedKeys:Ye(n,r),indeterminateKeys:Array.from(a)};let{levelTreeNodeMap:u}=t,d;d=i===void 0?r===void 0?it(n,t,l,!1):et(r,n,t,l):nt(i,n,t,l);let f=c===`parent`,p=c===`child`||s,m=d,h=new Set,g=Math.max.apply(null,Array.from(u.keys()));for(let e=g;e>=0;--e){let t=e===0,n=u.get(e);for(let e of n){if(e.isLeaf)continue;let{key:n,shallowLoaded:r}=e;if(p&&r&&e.children.forEach(e=>{!e.disabled&&!e.isLeaf&&e.shallowLoaded&&m.has(e.key)&&m.delete(e.key)}),e.disabled||!r)continue;let i=!0,a=!1,o=!0;for(let t of e.children){let e=t.key;if(!t.disabled){if(o&&=!1,m.has(e))a=!0;else if(h.has(e)){a=!0,i=!1;break}else if(i=!1,a)break}}i&&!o?(f&&e.children.forEach(e=>{!e.disabled&&m.has(e.key)&&m.delete(e.key)}),m.add(n)):a&&h.add(n),t&&p&&m.has(n)&&m.delete(n)}}return{checkedKeys:Array.from(m),indeterminateKeys:Array.from(h)}}function it(e,t,n,r){let{treeNodeMap:i,getChildren:a}=t,o=new Set,s=new Set(e);return e.forEach(e=>{let t=i.get(e);t!==void 0&&Re(t,e=>{if(e.disabled)return Le.STOP;let{key:t}=e;if(!o.has(t)&&(o.add(t),s.add(t),Ke(e.rawNode,a))){if(r)return Le.STOP;if(!n)throw new $e}})}),s}function at(e,{includeGroup:t=!1,includeSelf:n=!0},r){let i=r.treeNodeMap,a=e==null?null:i.get(e)??null,o={keyPath:[],treeNodePath:[],treeNode:a};if(a?.ignored)return o.treeNode=null,o;for(;a;)!a.ignored&&(t||!a.isGroup)&&o.treeNodePath.push(a),a=a.parent;return o.treeNodePath.reverse(),n||o.treeNodePath.pop(),o.keyPath=o.treeNodePath.map(e=>e.key),o}function ot(e){if(e.length===0)return null;let t=e[0];return t.isGroup||t.ignored||t.disabled?t.getNext():t}function st(e,t){let n=e.siblings,r=n.length,{index:i}=e;return t?n[(i+1)%r]:i===n.length-1?null:n[i+1]}function ct(e,t,{loop:n=!1,includeDisabled:r=!1}={}){let i=t===`prev`?lt:st,a={reverse:t===`prev`},o=!1,s=null;function c(t){if(t!==null){if(t===e){if(!o)o=!0;else if(!e.disabled&&!e.isGroup){s=e;return}}else if((!t.disabled||r)&&!t.ignored&&!t.isGroup){s=t;return}if(t.isGroup){let e=dt(t,a);e===null?c(i(t,n)):s=e}else{let e=i(t,!1);if(e!==null)c(e);else{let e=ut(t);e?.isGroup?c(i(e,n)):n&&c(i(t,!0))}}}}return c(e),s}function lt(e,t){let n=e.siblings,r=n.length,{index:i}=e;return t?n[(i-1+r)%r]:i===0?null:n[i-1]}function ut(e){return e.parent}function dt(e,t={}){let{reverse:n=!1}=t,{children:r}=e;if(r){let{length:e}=r,i=n?e-1:0,a=n?-1:e,o=n?-1:1;for(let e=i;e!==a;e+=o){let n=r[e];if(!n.disabled&&!n.ignored)if(n.isGroup){let e=dt(n,t);if(e!==null)return e}else return n}}return null}var ft={getChild(){return this.ignored?null:dt(this)},getParent(){let{parent:e}=this;return e?.isGroup?e.getParent():e},getNext(e={}){return ct(this,`next`,e)},getPrev(e={}){return ct(this,`prev`,e)}};function pt(e,t){let n=t?new Set(t):void 0,r=[];function i(e){e.forEach(e=>{r.push(e),!(e.isLeaf||!e.children||e.ignored)&&(e.isGroup||n===void 0||n.has(e.key))&&i(e.children)})}return i(e),r}function mt(e,t){let n=e.key;for(;t;){if(t.key===n)return!0;t=t.parent}return!1}function ht(e,t,n,r,i,a=null,o=0){let s=[];return e.forEach((c,l)=>{var u;let d=Object.create(r);if(d.rawNode=c,d.siblings=s,d.level=o,d.index=l,d.isFirstChild=l===0,d.isLastChild=l+1===e.length,d.parent=a,!d.ignored){let e=i(c);Array.isArray(e)&&(d.children=ht(e,t,n,r,i,d,o+1))}s.push(d),t.set(d.key,d),n.has(o)||n.set(o,[]),(u=n.get(o))==null||u.push(d)}),s}function gt(e,t={}){let n=new Map,r=new Map,{getDisabled:i=Ge,getIgnored:a=Ue,getIsGroup:o=Ze,getKey:s=He}=t,c=t.getChildren??Ve,l=t.ignoreEmptyChildren?e=>{let t=c(e);return Array.isArray(t)?t.length?t:null:t}:c,u=ht(e,n,r,Object.assign({get key(){return s(this.rawNode)},get disabled(){return i(this.rawNode)},get isGroup(){return o(this.rawNode)},get isLeaf(){return Be(this.rawNode,l)},get shallowLoaded(){return We(this.rawNode,l)},get ignored(){return a(this.rawNode)},contains(e){return mt(this,e)}},ft),l);function d(e){if(e==null)return null;let t=n.get(e);return t&&!t.isGroup&&!t.ignored?t:null}function f(e){if(e==null)return null;let t=n.get(e);return t&&!t.ignored?t:null}function p(e,t){let n=f(e);return n?n.getPrev(t):null}function m(e,t){let n=f(e);return n?n.getNext(t):null}function h(e){let t=f(e);return t?t.getParent():null}function g(e){let t=f(e);return t?t.getChild():null}let _={treeNodes:u,treeNodeMap:n,levelTreeNodeMap:r,maxLevel:Math.max(...r.keys()),getChildren:l,getFlattenedNodes(e){return pt(u,e)},getNode:d,getPrev:p,getNext:m,getParent:h,getChild:g,getFirstAvailableNode(){return ot(u)},getPath(e,t={}){return at(e,t,_)},getCheckedKeys(e,t={}){let{cascade:n=!0,leafOnly:r=!1,checkStrategy:i=`all`,allowNotLoaded:a=!1}=t;return rt({checkedKeys:qe(e),indeterminateKeys:Je(e),cascade:n,leafOnly:r,checkStrategy:i,allowNotLoaded:a},_)},check(e,t,n={}){let{cascade:r=!0,leafOnly:i=!1,checkStrategy:a=`all`,allowNotLoaded:o=!1}=n;return rt({checkedKeys:qe(t),indeterminateKeys:Je(t),keysToCheck:e==null?[]:Ie(e),cascade:r,leafOnly:i,checkStrategy:a,allowNotLoaded:o},_)},uncheck(e,t,n={}){let{cascade:r=!0,leafOnly:i=!1,checkStrategy:a=`all`,allowNotLoaded:o=!1}=n;return rt({checkedKeys:qe(t),indeterminateKeys:Je(t),keysToUncheck:e==null?[]:Ie(e),cascade:r,leafOnly:i,checkStrategy:a,allowNotLoaded:o},_)},getNonLeafKeys(e={}){return ze(u,e)}};return _}var _t=M({name:`NBaseSelectGroupHeader`,props:{clsPrefix:{type:String,required:!0},tmNode:{type:Object,required:!0}},setup(){let{renderLabelRef:e,renderOptionRef:t,labelFieldRef:n,nodePropsRef:r}=F(re);return{labelField:n,nodeProps:r,renderLabel:e,renderOption:t}},render(){let{clsPrefix:e,renderLabel:t,renderOption:n,nodeProps:r,tmNode:{rawNode:i}}=this,a=r?.(i),o=t?t(i,!1):Z(i[this.labelField],i,!1),s=S(`div`,Object.assign({},a,{class:[`${e}-base-select-group-header`,a?.class]}),o);return i.render?i.render({node:s,option:i}):n?n({node:s,option:i,selected:!1}):s}});function vt(e,t){return S(o,{name:`fade-in-scale-up-transition`},{default:()=>e?S(ce,{clsPrefix:t,class:`${t}-base-select-option__check`},{default:()=>S(Pe)}):null})}var yt=M({name:`NBaseSelectOption`,props:{clsPrefix:{type:String,required:!0},tmNode:{type:Object,required:!0}},setup(e){let{valueRef:t,pendingTmNodeRef:n,multipleRef:r,valueSetRef:i,renderLabelRef:a,renderOptionRef:o,labelFieldRef:s,valueFieldRef:c,showCheckmarkRef:l,nodePropsRef:u,handleOptionClick:d,handleOptionMouseEnter:f}=F(re),p=L(()=>{let{value:t}=n;return t?e.tmNode.key===t.key:!1});function m(t){let{tmNode:n}=e;n.disabled||d(t,n)}function h(t){let{tmNode:n}=e;n.disabled||f(t,n)}function g(t){let{tmNode:n}=e,{value:r}=p;n.disabled||r||f(t,n)}return{multiple:r,isGrouped:L(()=>{let{tmNode:t}=e,{parent:n}=t;return n&&n.rawNode.type===`group`}),showCheckmark:l,nodeProps:u,isPending:p,isSelected:L(()=>{let{value:n}=t,{value:a}=r;if(n===null)return!1;let o=e.tmNode.rawNode[c.value];if(a){let{value:e}=i;return e.has(o)}else return n===o}),labelField:s,renderLabel:a,renderOption:o,handleMouseMove:g,handleMouseEnter:h,handleClick:m}},render(){let{clsPrefix:e,tmNode:{rawNode:t},isSelected:n,isPending:r,isGrouped:i,showCheckmark:a,nodeProps:o,renderOption:s,renderLabel:c,handleClick:l,handleMouseEnter:u,handleMouseMove:d}=this,f=vt(n,e),p=c?[c(t,n),a&&f]:[Z(t[this.labelField],t,n),a&&f],m=o?.(t),h=S(`div`,Object.assign({},m,{class:[`${e}-base-select-option`,t.class,m?.class,{[`${e}-base-select-option--disabled`]:t.disabled,[`${e}-base-select-option--selected`]:n,[`${e}-base-select-option--grouped`]:i,[`${e}-base-select-option--pending`]:r,[`${e}-base-select-option--show-checkmark`]:a}],style:[m?.style||``,t.style||``],onClick:Ne([l,m?.onClick]),onMouseenter:Ne([u,m?.onMouseenter]),onMousemove:Ne([d,m?.onMousemove])}),S(`div`,{class:`${e}-base-select-option__content`},p));return t.render?t.render({node:h,option:t,selected:n}):s?s({node:h,option:t,selected:n}):h}}),bt=u(`base-select-menu`,`
 line-height: 1.5;
 outline: none;
 z-index: 0;
 position: relative;
 border-radius: var(--n-border-radius);
 transition:
 background-color .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier);
 background-color: var(--n-color);
`,[u(`scrollbar`,`
 max-height: var(--n-height);
 `),u(`virtual-list`,`
 max-height: var(--n-height);
 `),u(`base-select-option`,`
 min-height: var(--n-option-height);
 font-size: var(--n-option-font-size);
 display: flex;
 align-items: center;
 `,[p(`content`,`
 z-index: 1;
 white-space: nowrap;
 text-overflow: ellipsis;
 overflow: hidden;
 `)]),u(`base-select-group-header`,`
 min-height: var(--n-option-height);
 font-size: .93em;
 display: flex;
 align-items: center;
 `),u(`base-select-menu-option-wrapper`,`
 position: relative;
 width: 100%;
 `),p(`loading, empty`,`
 display: flex;
 padding: 12px 32px;
 flex: 1;
 justify-content: center;
 `),p(`loading`,`
 color: var(--n-loading-color);
 font-size: var(--n-loading-size);
 `),p(`header`,`
 padding: 8px var(--n-option-padding-left);
 font-size: var(--n-option-font-size);
 transition: 
 color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 border-bottom: 1px solid var(--n-action-divider-color);
 color: var(--n-action-text-color);
 `),p(`action`,`
 padding: 8px var(--n-option-padding-left);
 font-size: var(--n-option-font-size);
 transition: 
 color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 border-top: 1px solid var(--n-action-divider-color);
 color: var(--n-action-text-color);
 `),u(`base-select-group-header`,`
 position: relative;
 cursor: default;
 padding: var(--n-option-padding);
 color: var(--n-group-header-text-color);
 `),u(`base-select-option`,`
 cursor: pointer;
 position: relative;
 padding: var(--n-option-padding);
 transition:
 color .3s var(--n-bezier),
 opacity .3s var(--n-bezier);
 box-sizing: border-box;
 color: var(--n-option-text-color);
 opacity: 1;
 `,[i(`show-checkmark`,`
 padding-right: calc(var(--n-option-padding-right) + 20px);
 `),k(`&::before`,`
 content: "";
 position: absolute;
 left: 4px;
 right: 4px;
 top: 0;
 bottom: 0;
 border-radius: var(--n-border-radius);
 transition: background-color .3s var(--n-bezier);
 `),k(`&:active`,`
 color: var(--n-option-text-color-pressed);
 `),i(`grouped`,`
 padding-left: calc(var(--n-option-padding-left) * 1.5);
 `),i(`pending`,[k(`&::before`,`
 background-color: var(--n-option-color-pending);
 `)]),i(`selected`,`
 color: var(--n-option-text-color-active);
 `,[k(`&::before`,`
 background-color: var(--n-option-color-active);
 `),i(`pending`,[k(`&::before`,`
 background-color: var(--n-option-color-active-pending);
 `)])]),i(`disabled`,`
 cursor: not-allowed;
 `,[c(`selected`,`
 color: var(--n-option-text-color-disabled);
 `),i(`selected`,`
 opacity: var(--n-option-opacity-disabled);
 `)]),p(`check`,`
 font-size: 16px;
 position: absolute;
 right: calc(var(--n-option-padding-right) - 4px);
 top: calc(50% - 7px);
 color: var(--n-option-check-color);
 transition: color .3s var(--n-bezier);
 `,[ge({enterScale:`0.5`})])])]),xt=M({name:`InternalSelectMenu`,props:Object.assign(Object.assign({},h.props),{clsPrefix:{type:String,required:!0},scrollable:{type:Boolean,default:!0},treeMate:{type:Object,required:!0},multiple:Boolean,size:{type:String,default:`medium`},value:{type:[String,Number,Array],default:null},autoPending:Boolean,virtualScroll:{type:Boolean,default:!0},show:{type:Boolean,default:!0},labelField:{type:String,default:`label`},valueField:{type:String,default:`value`},loading:Boolean,focusable:Boolean,renderLabel:Function,renderOption:Function,nodeProps:Function,showCheckmark:{type:Boolean,default:!0},onMousedown:Function,onScroll:Function,onFocus:Function,onBlur:Function,onKeyup:Function,onKeydown:Function,onTabOut:Function,onMouseenter:Function,onMouseleave:Function,onResize:Function,resetMenuOnOptionsChange:{type:Boolean,default:!0},inlineThemeDisabled:Boolean,scrollbarProps:Object,onToggle:Function}),setup(n){let{mergedClsPrefixRef:i,mergedRtlRef:a,mergedComponentPropsRef:o}=ee(n),c=d(`InternalSelectMenu`,a,i),u=h(`InternalSelectMenu`,`-internal-select-menu`,bt,pe,n,t(n,`clsPrefix`)),p=s(null),m=s(null),g=s(null),_=e(()=>n.treeMate.getFlattenedNodes()),y=e(()=>Qe(_.value)),x=s(null);function S(){let{treeMate:e}=n,t=null,{value:r}=n;r===null?t=e.getFirstAvailableNode():(t=n.multiple?e.getNode((r||[])[(r||[]).length-1]):e.getNode(r),(!t||t.disabled)&&(t=e.getFirstAvailableNode())),H(t||null)}function C(){let{value:e}=x;e&&!n.treeMate.getNode(e.key)&&(x.value=null)}let w;I(()=>n.show,e=>{e?w=I(()=>n.treeMate,()=>{n.resetMenuOnOptionsChange?(n.autoPending?S():C(),T(U)):C()},{immediate:!0}):w?.()},{immediate:!0}),O(()=>{w?.()});let E=e(()=>r(u.value.self[l(`optionHeight`,n.size)])),D=e(()=>f(u.value.self[l(`padding`,n.size)])),k=e(()=>n.multiple&&Array.isArray(n.value)?new Set(n.value):new Set),A=e(()=>{let e=_.value;return e&&e.length===0}),j=e(()=>o?.value?.Select?.renderEmpty);function te(e){let{onToggle:t}=n;t&&t(e)}function M(e){let{onScroll:t}=n;t&&t(e)}function N(e){var t;(t=g.value)==null||t.sync(),M(e)}function F(){var e;(e=g.value)==null||e.sync()}function L(){let{value:e}=x;return e||null}function ne(e,t){t.disabled||H(t,!1)}function R(e,t){t.disabled||te(t)}function z(e){var t;Q(e,`action`)||(t=n.onKeyup)==null||t.call(n,e)}function ie(e){var t;Q(e,`action`)||(t=n.onKeydown)==null||t.call(n,e)}function ae(e){var t;(t=n.onMousedown)==null||t.call(n,e),!n.focusable&&e.preventDefault()}function V(){let{value:e}=x;e&&H(e.getNext({loop:!0}),!0)}function oe(){let{value:e}=x;e&&H(e.getPrev({loop:!0}),!0)}function H(e,t=!1){x.value=e,t&&U()}function U(){var e,t;let r=x.value;if(!r)return;let i=y.value(r.key);i!==null&&(n.virtualScroll?(e=m.value)==null||e.scrollTo({index:i}):(t=g.value)==null||t.scrollTo({index:i,elSize:E.value}))}function W(e){var t;p.value?.contains(e.target)&&((t=n.onFocus)==null||t.call(n,e))}function G(e){var t;p.value?.contains(e.relatedTarget)||(t=n.onBlur)==null||t.call(n,e)}b(re,{handleOptionMouseEnter:ne,handleOptionClick:R,valueSetRef:k,pendingTmNodeRef:x,nodePropsRef:t(n,`nodeProps`),showCheckmarkRef:t(n,`showCheckmark`),multipleRef:t(n,`multiple`),valueRef:t(n,`value`),renderLabelRef:t(n,`renderLabel`),renderOptionRef:t(n,`renderOption`),labelFieldRef:t(n,`labelField`),valueFieldRef:t(n,`valueField`)}),b(B,p),v(()=>{let{value:e}=g;e&&e.sync()});let K=e(()=>{let{size:e}=n,{common:{cubicBezierEaseInOut:t},self:{height:r,borderRadius:i,color:a,groupHeaderTextColor:o,actionDividerColor:s,optionTextColorPressed:c,optionTextColor:d,optionTextColorDisabled:p,optionTextColorActive:m,optionOpacityDisabled:h,optionCheckColor:g,actionTextColor:_,optionColorPending:v,optionColorActive:y,loadingColor:b,loadingSize:x,optionColorActivePending:S,[l(`optionFontSize`,e)]:C,[l(`optionHeight`,e)]:w,[l(`optionPadding`,e)]:T}}=u.value;return{"--n-height":r,"--n-action-divider-color":s,"--n-action-text-color":_,"--n-bezier":t,"--n-border-radius":i,"--n-color":a,"--n-option-font-size":C,"--n-group-header-text-color":o,"--n-option-check-color":g,"--n-option-color-pending":v,"--n-option-color-active":y,"--n-option-color-active-pending":S,"--n-option-height":w,"--n-option-opacity-disabled":h,"--n-option-text-color":d,"--n-option-text-color-active":m,"--n-option-text-color-disabled":p,"--n-option-text-color-pressed":c,"--n-option-padding":T,"--n-option-padding-left":f(T,`left`),"--n-option-padding-right":f(T,`right`),"--n-loading-color":b,"--n-loading-size":x}}),{inlineThemeDisabled:se}=n,q=se?P(`internal-select-menu`,e(()=>n.size[0]),K,n):void 0,J={selfRef:p,next:V,prev:oe,getPendingTmNode:L};return je(p,n.onResize),Object.assign({mergedTheme:u,mergedClsPrefix:i,rtlEnabled:c,virtualListRef:m,scrollbarRef:g,itemSize:E,padding:D,flattenedNodes:_,empty:A,mergedRenderEmpty:j,virtualListContainer(){let{value:e}=m;return e?.listElRef},virtualListContent(){let{value:e}=m;return e?.itemsElRef},doScroll:M,handleFocusin:W,handleFocusout:G,handleKeyUp:z,handleKeyDown:ie,handleMouseDown:ae,handleVirtualListResize:F,handleVirtualListScroll:N,cssVars:se?void 0:K,themeClass:q?.themeClass,onRender:q?.onRender},J)},render(){let{$slots:e,virtualScroll:t,clsPrefix:n,mergedTheme:r,themeClass:i,onRender:a}=this;return a?.(),S(`div`,{ref:`selfRef`,tabindex:this.focusable?0:-1,class:[`${n}-base-select-menu`,`${n}-base-select-menu--${this.size}-size`,this.rtlEnabled&&`${n}-base-select-menu--rtl`,i,this.multiple&&`${n}-base-select-menu--multiple`],style:this.cssVars,onFocusin:this.handleFocusin,onFocusout:this.handleFocusout,onKeyup:this.handleKeyUp,onKeydown:this.handleKeyDown,onMousedown:this.handleMouseDown,onMouseenter:this.onMouseenter,onMouseleave:this.onMouseleave},w(e.header,e=>e&&S(`div`,{class:`${n}-base-select-menu__header`,"data-header":!0,key:`header`},e)),this.loading?S(`div`,{class:`${n}-base-select-menu__loading`},S(ne,{clsPrefix:n,strokeWidth:20})):this.empty?S(`div`,{class:`${n}-base-select-menu__empty`,"data-empty":!0},j(e.empty,()=>[this.mergedRenderEmpty?.call(this)||S(oe,{theme:r.peers.Empty,themeOverrides:r.peerOverrides.Empty,size:this.size})])):S(J,Object.assign({ref:`scrollbarRef`,theme:r.peers.Scrollbar,themeOverrides:r.peerOverrides.Scrollbar,scrollable:this.scrollable,container:t?this.virtualListContainer:void 0,content:t?this.virtualListContent:void 0,onScroll:t?void 0:this.doScroll},this.scrollbarProps),{default:()=>t?S(Oe,{ref:`virtualListRef`,class:`${n}-virtual-list`,items:this.flattenedNodes,itemSize:this.itemSize,showScrollbar:!1,paddingTop:this.padding.top,paddingBottom:this.padding.bottom,onResize:this.handleVirtualListResize,onScroll:this.handleVirtualListScroll,itemResizable:!0},{default:({item:e})=>e.isGroup?S(_t,{key:e.key,clsPrefix:n,tmNode:e}):e.ignored?null:S(yt,{clsPrefix:n,key:e.key,tmNode:e})}):S(`div`,{class:`${n}-base-select-menu-option-wrapper`,style:{paddingTop:this.padding.top,paddingBottom:this.padding.bottom}},this.flattenedNodes.map(e=>e.isGroup?S(_t,{key:e.key,clsPrefix:n,tmNode:e}):S(yt,{clsPrefix:n,key:e.key,tmNode:e})))}),w(e.action,e=>e&&[S(`div`,{class:`${n}-base-select-menu__action`,"data-action":!0,key:`action`},e),S(Fe,{onFocus:this.onTabOut,key:`focus-detector`})]))}}),St=k([u(`base-selection`,`
 --n-padding-single: var(--n-padding-single-top) var(--n-padding-single-right) var(--n-padding-single-bottom) var(--n-padding-single-left);
 --n-padding-multiple: var(--n-padding-multiple-top) var(--n-padding-multiple-right) var(--n-padding-multiple-bottom) var(--n-padding-multiple-left);
 position: relative;
 z-index: auto;
 box-shadow: none;
 width: 100%;
 max-width: 100%;
 display: inline-block;
 vertical-align: bottom;
 border-radius: var(--n-border-radius);
 min-height: var(--n-height);
 line-height: 1.5;
 font-size: var(--n-font-size);
 `,[u(`base-loading`,`
 color: var(--n-loading-color);
 `),u(`base-selection-tags`,`min-height: var(--n-height);`),p(`border, state-border`,`
 position: absolute;
 left: 0;
 right: 0;
 top: 0;
 bottom: 0;
 pointer-events: none;
 border: var(--n-border);
 border-radius: inherit;
 transition:
 box-shadow .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `),p(`state-border`,`
 z-index: 1;
 border-color: #0000;
 `),u(`base-suffix`,`
 cursor: pointer;
 position: absolute;
 top: 50%;
 transform: translateY(-50%);
 right: 10px;
 `,[p(`arrow`,`
 font-size: var(--n-arrow-size);
 color: var(--n-arrow-color);
 transition: color .3s var(--n-bezier);
 `)]),u(`base-selection-overlay`,`
 display: flex;
 align-items: center;
 white-space: nowrap;
 pointer-events: none;
 position: absolute;
 top: 0;
 right: 0;
 bottom: 0;
 left: 0;
 padding: var(--n-padding-single);
 transition: color .3s var(--n-bezier);
 `,[p(`wrapper`,`
 flex-basis: 0;
 flex-grow: 1;
 overflow: hidden;
 text-overflow: ellipsis;
 `)]),u(`base-selection-placeholder`,`
 color: var(--n-placeholder-color);
 `,[p(`inner`,`
 max-width: 100%;
 overflow: hidden;
 `)]),u(`base-selection-tags`,`
 cursor: pointer;
 outline: none;
 box-sizing: border-box;
 position: relative;
 z-index: auto;
 display: flex;
 padding: var(--n-padding-multiple);
 flex-wrap: wrap;
 align-items: center;
 width: 100%;
 vertical-align: bottom;
 background-color: var(--n-color);
 border-radius: inherit;
 transition:
 color .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 `),u(`base-selection-label`,`
 height: var(--n-height);
 display: inline-flex;
 width: 100%;
 vertical-align: bottom;
 cursor: pointer;
 outline: none;
 z-index: auto;
 box-sizing: border-box;
 position: relative;
 transition:
 color .3s var(--n-bezier),
 box-shadow .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 border-radius: inherit;
 background-color: var(--n-color);
 align-items: center;
 `,[u(`base-selection-input`,`
 font-size: inherit;
 line-height: inherit;
 outline: none;
 cursor: pointer;
 box-sizing: border-box;
 border:none;
 width: 100%;
 padding: var(--n-padding-single);
 background-color: #0000;
 color: var(--n-text-color);
 transition: color .3s var(--n-bezier);
 caret-color: var(--n-caret-color);
 `,[p(`content`,`
 text-overflow: ellipsis;
 overflow: hidden;
 white-space: nowrap; 
 `)]),p(`render-label`,`
 color: var(--n-text-color);
 `)]),c(`disabled`,[k(`&:hover`,[p(`state-border`,`
 box-shadow: var(--n-box-shadow-hover);
 border: var(--n-border-hover);
 `)]),i(`focus`,[p(`state-border`,`
 box-shadow: var(--n-box-shadow-focus);
 border: var(--n-border-focus);
 `)]),i(`active`,[p(`state-border`,`
 box-shadow: var(--n-box-shadow-active);
 border: var(--n-border-active);
 `),u(`base-selection-label`,`background-color: var(--n-color-active);`),u(`base-selection-tags`,`background-color: var(--n-color-active);`)])]),i(`disabled`,`cursor: not-allowed;`,[p(`arrow`,`
 color: var(--n-arrow-color-disabled);
 `),u(`base-selection-label`,`
 cursor: not-allowed;
 background-color: var(--n-color-disabled);
 `,[u(`base-selection-input`,`
 cursor: not-allowed;
 color: var(--n-text-color-disabled);
 `),p(`render-label`,`
 color: var(--n-text-color-disabled);
 `)]),u(`base-selection-tags`,`
 cursor: not-allowed;
 background-color: var(--n-color-disabled);
 `),u(`base-selection-placeholder`,`
 cursor: not-allowed;
 color: var(--n-placeholder-color-disabled);
 `)]),u(`base-selection-input-tag`,`
 height: calc(var(--n-height) - 6px);
 line-height: calc(var(--n-height) - 6px);
 outline: none;
 display: none;
 position: relative;
 margin-bottom: 3px;
 max-width: 100%;
 vertical-align: bottom;
 `,[p(`input`,`
 font-size: inherit;
 font-family: inherit;
 min-width: 1px;
 padding: 0;
 background-color: #0000;
 outline: none;
 border: none;
 max-width: 100%;
 overflow: hidden;
 width: 1em;
 line-height: inherit;
 cursor: pointer;
 color: var(--n-text-color);
 caret-color: var(--n-caret-color);
 `),p(`mirror`,`
 position: absolute;
 left: 0;
 top: 0;
 white-space: pre;
 visibility: hidden;
 user-select: none;
 -webkit-user-select: none;
 opacity: 0;
 `)]),[`warning`,`error`].map(e=>i(`${e}-status`,[p(`state-border`,`border: var(--n-border-${e});`),c(`disabled`,[k(`&:hover`,[p(`state-border`,`
 box-shadow: var(--n-box-shadow-hover-${e});
 border: var(--n-border-hover-${e});
 `)]),i(`active`,[p(`state-border`,`
 box-shadow: var(--n-box-shadow-active-${e});
 border: var(--n-border-active-${e});
 `),u(`base-selection-label`,`background-color: var(--n-color-active-${e});`),u(`base-selection-tags`,`background-color: var(--n-color-active-${e});`)]),i(`focus`,[p(`state-border`,`
 box-shadow: var(--n-box-shadow-focus-${e});
 border: var(--n-border-focus-${e});
 `)])])]))]),u(`base-selection-popover`,`
 margin-bottom: -3px;
 display: flex;
 flex-wrap: wrap;
 margin-right: -8px;
 `),u(`base-selection-tag-wrapper`,`
 max-width: 100%;
 display: inline-flex;
 padding: 0 7px 3px 0;
 `,[k(`&:last-child`,`padding-right: 0;`),u(`tag`,`
 font-size: 14px;
 max-width: 100%;
 `,[p(`content`,`
 line-height: 1.25;
 text-overflow: ellipsis;
 overflow: hidden;
 `)])])]),Ct=M({name:`InternalSelection`,props:Object.assign(Object.assign({},h.props),{clsPrefix:{type:String,required:!0},bordered:{type:Boolean,default:void 0},active:Boolean,pattern:{type:String,default:``},placeholder:String,selectedOption:{type:Object,default:null},selectedOptions:{type:Array,default:null},labelField:{type:String,default:`label`},valueField:{type:String,default:`value`},multiple:Boolean,filterable:Boolean,clearable:Boolean,disabled:Boolean,size:{type:String,default:`medium`},loading:Boolean,autofocus:Boolean,showArrow:{type:Boolean,default:!0},inputProps:Object,focused:Boolean,renderTag:Function,onKeydown:Function,onClick:Function,onBlur:Function,onFocus:Function,onDeleteOption:Function,maxTagCount:[String,Number],ellipsisTagPopoverProps:Object,onClear:Function,onPatternInput:Function,onPatternFocus:Function,onPatternBlur:Function,renderLabel:Function,status:String,inlineThemeDisabled:Boolean,ignoreComposition:{type:Boolean,default:!0},onResize:Function}),setup(n){let{mergedClsPrefixRef:r,mergedRtlRef:i}=ee(n),a=d(`InternalSelection`,i,r),o=s(null),c=s(null),u=s(null),p=s(null),m=s(null),_=s(null),y=s(null),b=s(null),x=s(null),S=s(null),C=s(!1),w=s(!1),E=s(!1),D=h(`InternalSelection`,`-internal-selection`,St,he,n,t(n,`clsPrefix`)),O=e(()=>n.clearable&&!n.disabled&&(E.value||n.active)),k=e(()=>n.selectedOption?n.renderTag?n.renderTag({option:n.selectedOption,handleClose:()=>{}}):n.renderLabel?n.renderLabel(n.selectedOption,!0):Z(n.selectedOption[n.labelField],n.selectedOption,!0):n.placeholder),A=e(()=>{let e=n.selectedOption;if(e)return e[n.labelField]}),j=e(()=>n.multiple?!!(Array.isArray(n.selectedOptions)&&n.selectedOptions.length):n.selectedOption!==null);function te(){var e;let{value:t}=o;if(t){let{value:r}=c;r&&(r.style.width=`${t.offsetWidth}px`,n.maxTagCount!==`responsive`&&((e=x.value)==null||e.sync({showAllItemsBeforeCalculate:!1})))}}function M(){let{value:e}=S;e&&(e.style.display=`none`)}function N(){let{value:e}=S;e&&(e.style.display=`inline-block`)}I(t(n,`active`),e=>{e||M()}),I(t(n,`pattern`),()=>{n.multiple&&T(te)});function F(e){let{onFocus:t}=n;t&&t(e)}function L(e){let{onBlur:t}=n;t&&t(e)}function ne(e){let{onDeleteOption:t}=n;t&&t(e)}function R(e){let{onClear:t}=n;t&&t(e)}function z(e){let{onPatternInput:t}=n;t&&t(e)}function re(e){(!e.relatedTarget||!u.value?.contains(e.relatedTarget))&&F(e)}function B(e){u.value?.contains(e.relatedTarget)||L(e)}function ie(e){R(e)}function ae(){E.value=!0}function V(){E.value=!1}function oe(e){!n.active||!n.filterable||e.target!==c.value&&e.preventDefault()}function H(e){ne(e)}let U=s(!1);function W(e){if(e.key===`Backspace`&&!U.value&&!n.pattern.length){let{selectedOptions:e}=n;e?.length&&H(e[e.length-1])}}let G=null;function K(e){let{value:t}=o;t&&(t.textContent=e.target.value,te()),n.ignoreComposition&&U.value?G=e:z(e)}function se(){U.value=!0}function q(){U.value=!1,n.ignoreComposition&&z(G),G=null}function J(e){var t;w.value=!0,(t=n.onPatternFocus)==null||t.call(n,e)}function Y(e){var t;w.value=!1,(t=n.onPatternBlur)==null||t.call(n,e)}function ce(){var e,t;if(n.filterable)w.value=!1,(e=_.value)==null||e.blur(),(t=c.value)==null||t.blur();else if(n.multiple){let{value:e}=p;e?.blur()}else{let{value:e}=m;e?.blur()}}function X(){var e,t,r;n.filterable?(w.value=!1,(e=_.value)==null||e.focus()):n.multiple?(t=p.value)==null||t.focus():(r=m.value)==null||r.focus()}function le(){let{value:e}=c;e&&(N(),e.focus())}function ue(){let{value:e}=c;e&&e.blur()}function de(e){let{value:t}=y;t&&t.setTextContent(`+${e}`)}function fe(){let{value:e}=b;return e}function pe(){return c.value}let me=null;function ge(){me!==null&&window.clearTimeout(me)}function _e(){n.active||(ge(),me=window.setTimeout(()=>{j.value&&(C.value=!0)},100))}function Q(){ge()}function ve(e){e||(ge(),C.value=!1)}I(j,e=>{e||(C.value=!1)}),v(()=>{g(()=>{let e=_.value;e&&(n.disabled?e.removeAttribute(`tabindex`):e.tabIndex=w.value?-1:0)})}),je(u,n.onResize);let{inlineThemeDisabled:ye}=n,be=e(()=>{let{size:e}=n,{common:{cubicBezierEaseInOut:t},self:{fontWeight:r,borderRadius:i,color:a,placeholderColor:o,textColor:s,paddingSingle:c,paddingMultiple:u,caretColor:d,colorDisabled:p,textColorDisabled:m,placeholderColorDisabled:h,colorActive:g,boxShadowFocus:_,boxShadowActive:v,boxShadowHover:y,border:b,borderFocus:x,borderHover:S,borderActive:C,arrowColor:w,arrowColorDisabled:T,loadingColor:E,colorActiveWarning:O,boxShadowFocusWarning:ee,boxShadowActiveWarning:k,boxShadowHoverWarning:A,borderWarning:j,borderFocusWarning:te,borderHoverWarning:M,borderActiveWarning:N,colorActiveError:P,boxShadowFocusError:F,boxShadowActiveError:I,boxShadowHoverError:L,borderError:ne,borderFocusError:R,borderHoverError:z,borderActiveError:re,clearColor:B,clearColorHover:ie,clearColorPressed:ae,clearSize:V,arrowSize:oe,[l(`height`,e)]:H,[l(`fontSize`,e)]:U}}=D.value,W=f(c),G=f(u);return{"--n-bezier":t,"--n-border":b,"--n-border-active":C,"--n-border-focus":x,"--n-border-hover":S,"--n-border-radius":i,"--n-box-shadow-active":v,"--n-box-shadow-focus":_,"--n-box-shadow-hover":y,"--n-caret-color":d,"--n-color":a,"--n-color-active":g,"--n-color-disabled":p,"--n-font-size":U,"--n-height":H,"--n-padding-single-top":W.top,"--n-padding-multiple-top":G.top,"--n-padding-single-right":W.right,"--n-padding-multiple-right":G.right,"--n-padding-single-left":W.left,"--n-padding-multiple-left":G.left,"--n-padding-single-bottom":W.bottom,"--n-padding-multiple-bottom":G.bottom,"--n-placeholder-color":o,"--n-placeholder-color-disabled":h,"--n-text-color":s,"--n-text-color-disabled":m,"--n-arrow-color":w,"--n-arrow-color-disabled":T,"--n-loading-color":E,"--n-color-active-warning":O,"--n-box-shadow-focus-warning":ee,"--n-box-shadow-active-warning":k,"--n-box-shadow-hover-warning":A,"--n-border-warning":j,"--n-border-focus-warning":te,"--n-border-hover-warning":M,"--n-border-active-warning":N,"--n-color-active-error":P,"--n-box-shadow-focus-error":F,"--n-box-shadow-active-error":I,"--n-box-shadow-hover-error":L,"--n-border-error":ne,"--n-border-focus-error":R,"--n-border-hover-error":z,"--n-border-active-error":re,"--n-clear-size":V,"--n-clear-color":B,"--n-clear-color-hover":ie,"--n-clear-color-pressed":ae,"--n-arrow-size":oe,"--n-font-weight":r}}),xe=ye?P(`internal-selection`,e(()=>n.size[0]),be,n):void 0;return{mergedTheme:D,mergedClearable:O,mergedClsPrefix:r,rtlEnabled:a,patternInputFocused:w,filterablePlaceholder:k,label:A,selected:j,showTagsPanel:C,isComposing:U,counterRef:y,counterWrapperRef:b,patternInputMirrorRef:o,patternInputRef:c,selfRef:u,multipleElRef:p,singleElRef:m,patternInputWrapperRef:_,overflowRef:x,inputTagElRef:S,handleMouseDown:oe,handleFocusin:re,handleClear:ie,handleMouseEnter:ae,handleMouseLeave:V,handleDeleteOption:H,handlePatternKeyDown:W,handlePatternInputInput:K,handlePatternInputBlur:Y,handlePatternInputFocus:J,handleMouseEnterCounter:_e,handleMouseLeaveCounter:Q,handleFocusout:B,handleCompositionEnd:q,handleCompositionStart:se,onPopoverUpdateShow:ve,focus:X,focusInput:le,blur:ce,blurInput:ue,updateCounter:de,getCounter:fe,getTail:pe,renderLabel:n.renderLabel,cssVars:ye?void 0:be,themeClass:xe?.themeClass,onRender:xe?.onRender}},render(){let{status:e,multiple:t,size:n,disabled:r,filterable:i,maxTagCount:a,bordered:o,clsPrefix:s,ellipsisTagPopoverProps:c,onRender:l,renderTag:u,renderLabel:d}=this;l?.();let f=a===`responsive`,p=typeof a==`number`,m=f||p,h=S(Y,null,{default:()=>S(fe,{clsPrefix:s,loading:this.loading,showArrow:this.showArrow,showClear:this.mergedClearable&&this.selected,onClear:this.handleClear},{default:()=>{var e;return(e=this.$slots).arrow?.call(e)}})}),g;if(t){let{labelField:e}=this,t=t=>S(`div`,{class:`${s}-base-selection-tag-wrapper`,key:t.value},u?u({option:t,handleClose:()=>{this.handleDeleteOption(t)}}):S(V,{size:n,closable:!t.disabled,disabled:r,onClose:()=>{this.handleDeleteOption(t)},internalCloseIsButtonTag:!1,internalCloseFocusable:!1},{default:()=>d?d(t,!0):Z(t[e],t,!0)})),o=()=>(p?this.selectedOptions.slice(0,a):this.selectedOptions).map(t),l=i?S(`div`,{class:`${s}-base-selection-input-tag`,ref:`inputTagElRef`,key:`__input-tag__`},S(`input`,Object.assign({},this.inputProps,{ref:`patternInputRef`,tabindex:-1,disabled:r,value:this.pattern,autofocus:this.autofocus,class:`${s}-base-selection-input-tag__input`,onBlur:this.handlePatternInputBlur,onFocus:this.handlePatternInputFocus,onKeydown:this.handlePatternKeyDown,onInput:this.handlePatternInputInput,onCompositionstart:this.handleCompositionStart,onCompositionend:this.handleCompositionEnd})),S(`span`,{ref:`patternInputMirrorRef`,class:`${s}-base-selection-input-tag__mirror`},this.pattern)):null,v=f?()=>S(`div`,{class:`${s}-base-selection-tag-wrapper`,ref:`counterWrapperRef`},S(V,{size:n,ref:`counterRef`,onMouseenter:this.handleMouseEnterCounter,onMouseleave:this.handleMouseLeaveCounter,disabled:r})):void 0,y;if(p){let e=this.selectedOptions.length-a;e>0&&(y=S(`div`,{class:`${s}-base-selection-tag-wrapper`,key:`__counter__`},S(V,{size:n,ref:`counterRef`,onMouseenter:this.handleMouseEnterCounter,disabled:r},{default:()=>`+${e}`})))}let b=f?i?S(Ae,{ref:`overflowRef`,updateCounter:this.updateCounter,getCounter:this.getCounter,getTail:this.getTail,style:{width:`100%`,display:`flex`,overflow:`hidden`}},{default:o,counter:v,tail:()=>l}):S(Ae,{ref:`overflowRef`,updateCounter:this.updateCounter,getCounter:this.getCounter,style:{width:`100%`,display:`flex`,overflow:`hidden`}},{default:o,counter:v}):p&&y?o().concat(y):o(),x=m?()=>S(`div`,{class:`${s}-base-selection-popover`},f?o():this.selectedOptions.map(t)):void 0,C=m?Object.assign({show:this.showTagsPanel,trigger:`hover`,overlap:!0,placement:`top`,width:`trigger`,onUpdateShow:this.onPopoverUpdateShow,theme:this.mergedTheme.peers.Popover,themeOverrides:this.mergedTheme.peerOverrides.Popover},c):null,w=!this.selected&&(!this.active||!this.pattern&&!this.isComposing)?S(`div`,{class:`${s}-base-selection-placeholder ${s}-base-selection-overlay`},S(`div`,{class:`${s}-base-selection-placeholder__inner`},this.placeholder)):null,T=i?S(`div`,{ref:`patternInputWrapperRef`,class:`${s}-base-selection-tags`},b,f?null:l,h):S(`div`,{ref:`multipleElRef`,class:`${s}-base-selection-tags`,tabindex:r?void 0:0},b,h);g=S(_,null,m?S(U,Object.assign({},C,{scrollable:!0,style:`max-height: calc(var(--v-target-height) * 6.6);`}),{trigger:()=>T,default:x}):T,w)}else if(i){let e=this.pattern||this.isComposing,t=this.active?!e:!this.selected,n=!this.active&&this.selected;g=S(`div`,{ref:`patternInputWrapperRef`,class:`${s}-base-selection-label`,title:this.patternInputFocused?void 0:Me(this.label)},S(`input`,Object.assign({},this.inputProps,{ref:`patternInputRef`,class:`${s}-base-selection-input`,value:this.active?this.pattern:``,placeholder:``,readonly:r,disabled:r,tabindex:-1,autofocus:this.autofocus,onFocus:this.handlePatternInputFocus,onBlur:this.handlePatternInputBlur,onInput:this.handlePatternInputInput,onCompositionstart:this.handleCompositionStart,onCompositionend:this.handleCompositionEnd})),n?S(`div`,{class:`${s}-base-selection-label__render-label ${s}-base-selection-overlay`,key:`input`},S(`div`,{class:`${s}-base-selection-overlay__wrapper`},u?u({option:this.selectedOption,handleClose:()=>{}}):d?d(this.selectedOption,!0):Z(this.label,this.selectedOption,!0))):null,t?S(`div`,{class:`${s}-base-selection-placeholder ${s}-base-selection-overlay`,key:`placeholder`},S(`div`,{class:`${s}-base-selection-overlay__wrapper`},this.filterablePlaceholder)):null,h)}else g=S(`div`,{ref:`singleElRef`,class:`${s}-base-selection-label`,tabindex:this.disabled?void 0:0},this.label===void 0?S(`div`,{class:`${s}-base-selection-placeholder ${s}-base-selection-overlay`,key:`placeholder`},S(`div`,{class:`${s}-base-selection-placeholder__inner`},this.placeholder)):S(`div`,{class:`${s}-base-selection-input`,title:Me(this.label),key:`input`},S(`div`,{class:`${s}-base-selection-input__content`},u?u({option:this.selectedOption,handleClose:()=>{}}):d?d(this.selectedOption,!0):Z(this.label,this.selectedOption,!0))),h);return S(`div`,{ref:`selfRef`,class:[`${s}-base-selection`,this.rtlEnabled&&`${s}-base-selection--rtl`,this.themeClass,e&&`${s}-base-selection--${e}-status`,{[`${s}-base-selection--active`]:this.active,[`${s}-base-selection--selected`]:this.selected||this.active&&this.pattern,[`${s}-base-selection--disabled`]:this.disabled,[`${s}-base-selection--multiple`]:this.multiple,[`${s}-base-selection--focus`]:this.focused}],style:this.cssVars,onClick:this.onClick,onMouseenter:this.handleMouseEnter,onMouseleave:this.handleMouseLeave,onKeydown:this.onKeydown,onFocusin:this.handleFocusin,onFocusout:this.handleFocusout,onMousedown:this.handleMouseDown},g,o?S(`div`,{class:`${s}-base-selection__border`}):null,o?S(`div`,{class:`${s}-base-selection__state-border`}):null)}});function wt(e){return e.type===`group`}function Tt(e){return e.type===`ignored`}function Et(e,t){try{return!!(1+t.toString().toLowerCase().indexOf(e.trim().toLowerCase()))}catch{return!1}}function Dt(e,t){return{getIsGroup:wt,getIgnored:Tt,getKey(t){return wt(t)?t.name||t.key||`key-required`:t[e]},getChildren(e){return e[t]}}}function Ot(e,t,n,r){if(!t)return e;function i(e){if(!Array.isArray(e))return[];let a=[];for(let o of e)if(wt(o)){let e=i(o[r]);e.length&&a.push(Object.assign({},o,{[r]:e}))}else if(Tt(o))continue;else t(n,o)&&a.push(o);return a}return i(e)}function kt(e,t,n){let r=new Map;return e.forEach(e=>{wt(e)?e[n].forEach(e=>{r.set(e[t],e)}):r.set(e[t],e)}),r}var At=k([u(`select`,`
 z-index: auto;
 outline: none;
 width: 100%;
 position: relative;
 font-weight: var(--n-font-weight);
 `),u(`select-menu`,`
 margin: 4px 0;
 box-shadow: var(--n-menu-box-shadow);
 `,[ge({originalTransition:`background-color .3s var(--n-bezier), box-shadow .3s var(--n-bezier)`})])]),jt=M({name:`Select`,props:Object.assign(Object.assign({},h.props),{to:ae.propTo,bordered:{type:Boolean,default:void 0},clearable:Boolean,clearCreatedOptionsOnClear:{type:Boolean,default:!0},clearFilterAfterSelect:{type:Boolean,default:!0},options:{type:Array,default:()=>[]},defaultValue:{type:[String,Number,Array],default:null},keyboard:{type:Boolean,default:!0},value:[String,Number,Array],placeholder:String,menuProps:Object,multiple:Boolean,size:String,menuSize:{type:String},filterable:Boolean,disabled:{type:Boolean,default:void 0},remote:Boolean,loading:Boolean,filter:Function,placement:{type:String,default:`bottom-start`},widthMode:{type:String,default:`trigger`},tag:Boolean,onCreate:Function,fallbackOption:{type:[Function,Boolean],default:void 0},show:{type:Boolean,default:void 0},showArrow:{type:Boolean,default:!0},maxTagCount:[Number,String],ellipsisTagPopoverProps:Object,consistentMenuWidth:{type:Boolean,default:!0},virtualScroll:{type:Boolean,default:!0},labelField:{type:String,default:`label`},valueField:{type:String,default:`value`},childrenField:{type:String,default:`children`},renderLabel:Function,renderOption:Function,renderTag:Function,"onUpdate:value":[Function,Array],inputProps:Object,nodeProps:Function,ignoreComposition:{type:Boolean,default:!0},showOnFocus:Boolean,onUpdateValue:[Function,Array],onBlur:[Function,Array],onClear:[Function,Array],onFocus:[Function,Array],onScroll:[Function,Array],onSearch:[Function,Array],onUpdateShow:[Function,Array],"onUpdate:show":[Function,Array],displayDirective:{type:String,default:`show`},resetMenuOnOptionsChange:{type:Boolean,default:!0},status:String,showCheckmark:{type:Boolean,default:!0},scrollbarProps:Object,onChange:[Function,Array],items:Array}),slots:Object,setup(n){let{mergedClsPrefixRef:r,mergedBorderedRef:i,namespaceRef:a,inlineThemeDisabled:o,mergedComponentPropsRef:c}=ee(n),l=h(`Select`,`-select`,At,_e,n,r),u=s(n.defaultValue),d=ue(t(n,`value`),u),f=s(!1),p=s(``),m=de(n,[`items`,`options`]),g=s([]),_=s([]),v=e(()=>_.value.concat(g.value).concat(m.value)),y=e(()=>{let{filter:e}=n;if(e)return e;let{labelField:t,valueField:r}=n;return(e,n)=>{if(!n)return!1;let i=n[t];if(typeof i==`string`)return Et(e,i);let a=n[r];return typeof a==`string`?Et(e,a):typeof a==`number`&&Et(e,String(a))}}),b=e(()=>{if(n.remote)return m.value;{let{value:e}=v,{value:t}=p;return!t.length||!n.filterable?e:Ot(e,y.value,t,n.childrenField)}}),S=e(()=>{let{valueField:e,childrenField:t}=n,r=Dt(e,t);return gt(b.value,r)}),C=e(()=>kt(v.value,n.valueField,n.childrenField)),w=s(!1),T=ue(t(n,`show`),w),D=s(null),O=s(null),k=s(null),{localeRef:A}=le(`Select`),j=e(()=>n.placeholder??A.value.placeholder),M=[],N=s(new Map),F=e(()=>{let{fallbackOption:e}=n;if(e===void 0){let{labelField:e,valueField:t}=n;return n=>({[e]:String(n),[t]:n})}return e===!1?!1:t=>Object.assign(e(t),{value:t})});function L(e){let t=n.remote,{value:r}=N,{value:i}=C,{value:a}=F,o=[];return e.forEach(e=>{if(i.has(e))o.push(i.get(e));else if(t&&r.has(e))o.push(r.get(e));else if(a){let t=a(e);t&&o.push(t)}}),o}let ne=e(()=>{if(n.multiple){let{value:e}=d;return Array.isArray(e)?L(e):[]}return null}),R=e(()=>{let{value:e}=d;return!n.multiple&&!Array.isArray(e)?e===null?null:L([e])[0]||null:null}),z=te(n,{mergedSize:e=>{let{size:t}=n;if(t)return t;let{mergedSize:r}=e||{};return r?.value?r.value:c?.value?.Select?.size||`medium`}}),{mergedSizeRef:re,mergedDisabledRef:B,mergedStatusRef:ie}=z;function V(e,t){let{onChange:r,"onUpdate:value":i,onUpdateValue:a}=n,{nTriggerFormChange:o,nTriggerFormInput:s}=z;r&&E(r,e,t),a&&E(a,e,t),i&&E(i,e,t),u.value=e,o(),s()}function oe(e){let{onBlur:t}=n,{nTriggerFormBlur:r}=z;t&&E(t,e),r()}function H(){let{onClear:e}=n;e&&E(e)}function U(e){let{onFocus:t,showOnFocus:r}=n,{nTriggerFormFocus:i}=z;t&&E(t,e),i(),r&&J()}function W(e){let{onSearch:t}=n;t&&E(t,e)}function G(e){let{onScroll:t}=n;t&&E(t,e)}function K(){var e;let{remote:t,multiple:r}=n;if(t){let{value:t}=N;if(r){let{valueField:r}=n;(e=ne.value)==null||e.forEach(e=>{t.set(e[r],e)})}else{let e=R.value;e&&t.set(e[n.valueField],e)}}}function q(e){let{onUpdateShow:t,"onUpdate:show":r}=n;t&&E(t,e),r&&E(r,e),w.value=e}function J(){B.value||(q(!0),w.value=!0,n.filterable&&je())}function Y(){q(!1)}function ce(){p.value=``,_.value=M}let X=s(!1);function fe(){n.filterable&&(X.value=!0)}function pe(){n.filterable&&(X.value=!1,T.value||ce())}function he(){B.value||(T.value?n.filterable?je():Y():J())}function Z(e){(k.value?.selfRef)?.contains(e.relatedTarget)||(f.value=!1,oe(e),Y())}function ge(e){U(e),f.value=!0}function ve(){f.value=!0}function ye(e){D.value?.$el.contains(e.relatedTarget)||(f.value=!1,oe(e),Y())}function be(){var e;(e=D.value)==null||e.focus(),Y()}function xe(e){T.value&&(D.value?.$el.contains(se(e))||Y())}function Se(e){if(!Array.isArray(e))return[];if(F.value)return Array.from(e);{let{remote:t}=n,{value:r}=C;if(t){let{value:t}=N;return e.filter(e=>r.has(e)||t.has(e))}else return e.filter(e=>r.has(e))}}function Ce(e){we(e.rawNode)}function we(e){if(B.value)return;let{tag:t,remote:r,clearFilterAfterSelect:i,valueField:a}=n;if(t&&!r){let{value:e}=_,t=e[0]||null;if(t){let e=g.value;e.length?e.push(t):g.value=[t],_.value=M}}if(r&&N.value.set(e[a],e),n.multiple){let n=Se(d.value),o=n.findIndex(t=>t===e[a]);if(~o){if(n.splice(o,1),t&&!r){let t=Te(e[a]);~t&&(g.value.splice(t,1),i&&(p.value=``))}}else n.push(e[a]),i&&(p.value=``);V(n,L(n))}else{if(t&&!r){let t=Te(e[a]);~t?g.value=[g.value[t]]:g.value=M}Ae(),Y(),V(e[a],e)}}function Te(e){return g.value.findIndex(t=>t[n.valueField]===e)}function Ee(e){T.value||J();let{value:t}=e.target;p.value=t;let{tag:r,remote:i}=n;if(W(t),r&&!i){if(!t){_.value=M;return}let{onCreate:e}=n,r=e?e(t):{[n.labelField]:t,[n.valueField]:t},{valueField:i,labelField:a}=n;m.value.some(e=>e[i]===r[i]||e[a]===r[a])||g.value.some(e=>e[i]===r[i]||e[a]===r[a])?_.value=M:_.value=[r]}}function De(e){e.stopPropagation();let{multiple:t,tag:r,remote:i,clearCreatedOptionsOnClear:a}=n;!t&&n.filterable&&Y(),r&&!i&&a&&(g.value=M),H(),t?V([],[]):V(null,null)}function Oe(e){!Q(e,`action`)&&!Q(e,`empty`)&&!Q(e,`header`)&&e.preventDefault()}function $(e){G(e)}function ke(e){var t,r,i;if(!n.keyboard){e.preventDefault();return}switch(e.key){case` `:if(n.filterable)break;e.preventDefault();case`Enter`:if(!D.value?.isComposing){if(T.value){let e=k.value?.getPendingTmNode();e?Ce(e):n.filterable||(Y(),Ae())}else if(J(),n.tag&&X.value){let e=_.value[0];if(e){let t=e[n.valueField],{value:r}=d;n.multiple&&Array.isArray(r)&&r.includes(t)||we(e)}}}e.preventDefault();break;case`ArrowUp`:if(e.preventDefault(),n.loading)return;T.value&&((t=k.value)==null||t.prev());break;case`ArrowDown`:if(e.preventDefault(),n.loading)return;T.value?(r=k.value)==null||r.next():J();break;case`Escape`:T.value&&(me(e),Y()),(i=D.value)==null||i.focus();break}}function Ae(){var e;(e=D.value)==null||e.focus()}function je(){var e;(e=D.value)==null||e.focusInput()}function Me(){var e;T.value&&((e=O.value)==null||e.syncPosition())}K(),I(t(n,`options`),K);let Ne={focus:()=>{var e;(e=D.value)==null||e.focus()},focusInput:()=>{var e;(e=D.value)==null||e.focusInput()},blur:()=>{var e;(e=D.value)==null||e.blur()},blurInput:()=>{var e;(e=D.value)==null||e.blurInput()}},Pe=e(()=>{let{self:{menuBoxShadow:e}}=l.value;return{"--n-menu-box-shadow":e}}),Fe=o?P(`select`,void 0,Pe,n):void 0;return Object.assign(Object.assign({},Ne),{mergedStatus:ie,mergedClsPrefix:r,mergedBordered:i,namespace:a,treeMate:S,isMounted:x(),triggerRef:D,menuRef:k,pattern:p,uncontrolledShow:w,mergedShow:T,adjustedTo:ae(n),uncontrolledValue:u,mergedValue:d,followerRef:O,localizedPlaceholder:j,selectedOption:R,selectedOptions:ne,mergedSize:re,mergedDisabled:B,focused:f,activeWithoutMenuOpen:X,inlineThemeDisabled:o,onTriggerInputFocus:fe,onTriggerInputBlur:pe,handleTriggerOrMenuResize:Me,handleMenuFocus:ve,handleMenuBlur:ye,handleMenuTabOut:be,handleTriggerClick:he,handleToggle:Ce,handleDeleteOption:we,handlePatternInput:Ee,handleClear:De,handleTriggerBlur:Z,handleTriggerFocus:ge,handleKeydown:ke,handleMenuAfterLeave:ce,handleMenuClickOutside:xe,handleMenuScroll:$,handleMenuKeydown:ke,handleMenuMousedown:Oe,mergedTheme:l,cssVars:o?void 0:Pe,themeClass:Fe?.themeClass,onRender:Fe?.onRender})},render(){return S(`div`,{class:`${this.mergedClsPrefix}-select`},S(H,null,{default:()=>[S(z,null,{default:()=>S(Ct,{ref:`triggerRef`,inlineThemeDisabled:this.inlineThemeDisabled,status:this.mergedStatus,inputProps:this.inputProps,clsPrefix:this.mergedClsPrefix,showArrow:this.showArrow,maxTagCount:this.maxTagCount,ellipsisTagPopoverProps:this.ellipsisTagPopoverProps,bordered:this.mergedBordered,active:this.activeWithoutMenuOpen||this.mergedShow,pattern:this.pattern,placeholder:this.localizedPlaceholder,selectedOption:this.selectedOption,selectedOptions:this.selectedOptions,multiple:this.multiple,renderTag:this.renderTag,renderLabel:this.renderLabel,filterable:this.filterable,clearable:this.clearable,disabled:this.mergedDisabled,size:this.mergedSize,theme:this.mergedTheme.peers.InternalSelection,labelField:this.labelField,valueField:this.valueField,themeOverrides:this.mergedTheme.peerOverrides.InternalSelection,loading:this.loading,focused:this.focused,onClick:this.handleTriggerClick,onDeleteOption:this.handleDeleteOption,onPatternInput:this.handlePatternInput,onClear:this.handleClear,onBlur:this.handleTriggerBlur,onFocus:this.handleTriggerFocus,onKeydown:this.handleKeydown,onPatternBlur:this.onTriggerInputBlur,onPatternFocus:this.onTriggerInputFocus,onResize:this.handleTriggerOrMenuResize,ignoreComposition:this.ignoreComposition},{arrow:()=>{var e;return[(e=this.$slots).arrow?.call(e)]}})}),S(ie,{ref:`followerRef`,show:this.mergedShow,to:this.adjustedTo,teleportDisabled:this.adjustedTo===ae.tdkey,containerClass:this.namespace,width:this.consistentMenuWidth?`target`:void 0,minWidth:`target`,placement:this.placement},{default:()=>S(o,{name:`fade-in-scale-up-transition`,appear:this.isMounted,onAfterLeave:this.handleMenuAfterLeave},{default:()=>{var e;return this.mergedShow||this.displayDirective===`show`?((e=this.onRender)==null||e.call(this),n(S(xt,Object.assign({},this.menuProps,{ref:`menuRef`,onResize:this.handleTriggerOrMenuResize,inlineThemeDisabled:this.inlineThemeDisabled,virtualScroll:this.consistentMenuWidth&&this.virtualScroll,class:[`${this.mergedClsPrefix}-select-menu`,this.themeClass,this.menuProps?.class],clsPrefix:this.mergedClsPrefix,focusable:!0,labelField:this.labelField,valueField:this.valueField,autoPending:!0,nodeProps:this.nodeProps,theme:this.mergedTheme.peers.InternalSelectMenu,themeOverrides:this.mergedTheme.peerOverrides.InternalSelectMenu,treeMate:this.treeMate,multiple:this.multiple,size:this.menuSize,renderOption:this.renderOption,renderLabel:this.renderLabel,value:this.mergedValue,style:[this.menuProps?.style,this.cssVars],onToggle:this.handleToggle,onScroll:this.handleMenuScroll,onFocus:this.handleMenuFocus,onBlur:this.handleMenuBlur,onKeydown:this.handleMenuKeydown,onTabOut:this.handleMenuTabOut,onMousedown:this.handleMenuMousedown,show:this.mergedShow,showCheckmark:this.showCheckmark,resetMenuOnOptionsChange:this.resetMenuOnOptionsChange,scrollbarProps:this.scrollbarProps}),{empty:()=>{var e;return[(e=this.$slots).empty?.call(e)]},header:()=>{var e;return[(e=this.$slots).header?.call(e)]},action:()=>{var e;return[(e=this.$slots).action?.call(e)]}}),this.displayDirective===`show`?[[a,this.mergedShow],[X,this.handleMenuClickOutside,void 0,{capture:!0}]]:[[X,this.handleMenuClickOutside,void 0,{capture:!0}]])):null}})})]}))}});export{Ne as a,gt as i,Dt as n,Oe as o,xt as r,Q as s,jt as t};