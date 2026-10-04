var appTheme='light';
function initTheme(){
  var saved=null;try{saved=localStorage.getItem('prism_theme')}catch(e){}
  appTheme=saved||'light';
  document.documentElement.setAttribute('data-theme',appTheme);
  if(typeof refreshThemePickerLabel==='function'){refreshThemePickerLabel()}else{
    var tp=document.getElementById('cfg-theme-picker');if(tp){var lbs={'light':'浅色 — 暖黄','dark':'深色 — 蓝灰','forest':'墨绿 — 自然','amethyst':'暗紫 — 优雅','ocean':'海洋蓝','sakura':'樱花粉','midnight':'午夜深蓝','sepia':'复古棕'};tp.setAttribute('data-value',appTheme);tp.querySelector('.picker-label').textContent=lbs[appTheme]||'浅色 — 暖黄'}
  }
  syncStatusBarTheme()
}
function setTheme(t){
  appTheme=t||'light';
  document.documentElement.setAttribute('data-theme',appTheme);
  try{localStorage.setItem('prism_theme',appTheme)}catch(e){}
  // 切换内建主题时清掉自定义主题的内联 CSS 变量
  if(appTheme!=='custom'){
    ['--phx-primary','--phx-primary-hover','--phx-primary-active','--phx-primary-bg','--phx-bg','--phx-bg-card','--phx-text','--phx-text-secondary','--phx-border','--phx-border-light'].forEach(function(k){
      document.documentElement.style.removeProperty(k)
    });
  }
  var names={light:'浅色暖黄',dark:'深色蓝灰',forest:'墨绿自然',amethyst:'暗紫优雅',ocean:'海洋蓝',sakura:'樱花粉',midnight:'午夜深蓝',sepia:'复古棕',custom:'自定义'};
  T('已切换: '+(names[appTheme]||appTheme),'o');
  if(typeof refreshThemePickerLabel==='function')refreshThemePickerLabel();
  syncStatusBarTheme()
}
function syncStatusBarTheme(){
  if(typeof android=='undefined')return;
  var isDark=appTheme==='dark'||appTheme==='amethyst'||appTheme==='ocean'||appTheme==='sakura'||appTheme==='midnight';
  if(appTheme==='custom'){
    try{var ct=JSON.parse(localStorage.getItem('prism_custom_theme'));if(ct)isDark=ct.dark!==false}catch(e){}
  }
  if(android.setLightStatusBar){
    android.setLightStatusBar(isDark)
  }
  if(android.setStatusBarColor){
    var colors={light:'#f8f8f0',dark:'#0f1720',forest:'#0d1a10',amethyst:'#0d0818',ocean:'#0c1929',sakura:'#1a0f18',midnight:'#0a0e17',sepia:'#f5f0e8'};
    if(appTheme==='custom'){
      try{var ct=JSON.parse(localStorage.getItem('prism_custom_theme'));if(ct)android.setStatusBarColor(ct.background);else android.setStatusBarColor(colors.light)}catch(e){android.setStatusBarColor(colors.light)}
    }else{
      android.setStatusBarColor(colors[appTheme]||colors.light)
    }
  }
}
initTheme();
