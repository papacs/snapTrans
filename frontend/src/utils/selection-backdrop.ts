import type { Rect } from "./selection";

// UIA reports glyph bounds, while browsers paint selection highlights across
// the taller line box. Extend only across uniform highlight-colored strips;
// never pad blindly into adjacent, unselected text.
export function selectionBackdrop(
  pixels: Pick<ImageData, "data" | "width" | "height">,
  text: Rect,
  background: string
): Rect | null {
  if (!/^#[0-9a-f]{6}$/i.test(background)) return null;
  const {data,width,height}=pixels;
  if (data.length < width*height*4 || width < 1 || height < 1) return null;
  const x0=Math.max(0,Math.floor(text.x)), y0=Math.max(0,Math.floor(text.y));
  const x1=Math.min(width,Math.ceil(text.x+text.width)), y1=Math.min(height,Math.ceil(text.y+text.height));
  if(x1<=x0 || y1<=y0) return null;
  const counts=new Map<number,number>();
  for(let y=y0;y<y1;y++) for(let x=x0;x<x1;x++){
    const i=(y*width+x)*4;
    const color=(data[i]!<<16)|(data[i+1]!<<8)|data[i+2]!;
    counts.set(color,(counts.get(color)??0)+1);
  }
  const dominant=[...counts].sort((a,b)=>b[1]-a[1])[0];
  if(!dominant || dominant[1]<(x1-x0)*(y1-y0)*.45) return null;
  const color=dominant[0], bg=parseInt(background.slice(1),16);
  const channel=(c:number,s:number)=>(c>>s)&255;
  if([16,8,0].every(s=>Math.abs(channel(color,s)-channel(bg,s))<35)) return null;
  const matches=(x:number,y:number)=>{
    const i=(y*width+x)*4;
    return Math.abs(data[i]!-channel(color,16))<8 && Math.abs(data[i+1]!-channel(color,8))<8 && Math.abs(data[i+2]!-channel(color,0))<8;
  };
  const row=(y:number)=>{let n=0;for(let x=x0;x<x1;x++)if(matches(x,y))n++;return n/(x1-x0)>.94;};
  let top=y0,bottom=y1,left=x0,right=x1;
  const pad=Math.ceil(text.height*.7);
  while(top>Math.max(0,y0-pad)&&row(top-1))top--;
  while(bottom<Math.min(height,y1+pad)&&row(bottom))bottom++;
  const column=(x:number)=>{let n=0;for(let y=top;y<bottom;y++)if(matches(x,y))n++;return n/(bottom-top)>.94;};
  while(left>Math.max(0,x0-pad)&&column(left-1))left--;
  while(right<Math.min(width,x1+pad)&&column(right))right++;
  if(top===y0&&bottom===y1&&left===x0&&right===x1)return null;
  return {x:left,y:top,width:right-left,height:bottom-top};
}

export function nativeSelectionBackdrop(canvas: HTMLCanvasElement|null,rect:Rect,background?:string): Rect|null {
  if(!canvas || !background) return null;
  const bounds=canvas.getBoundingClientRect();
  if(!bounds.width||!bounds.height)return null;
  const sx=canvas.width/bounds.width,sy=canvas.height/bounds.height;
  const text={x:(rect.x-bounds.left)*sx,y:(rect.y-bounds.top)*sy,width:rect.width*sx,height:rect.height*sy};
  const pad=Math.ceil(text.height*.7)+1;
  const x=Math.max(0,Math.floor(text.x-pad)),y=Math.max(0,Math.floor(text.y-pad));
  const width=Math.min(canvas.width-x,Math.ceil(text.width+pad*2)),height=Math.min(canvas.height-y,Math.ceil(text.height+pad*2));
  if(width<1||height<1)return null;
  try{
    const context=canvas.getContext("2d");
    if(!context)return null;
    const mask=selectionBackdrop(context.getImageData(x,y,width,height),{...text,x:text.x-x,y:text.y-y},background);
    return mask?{x:(mask.x+x)/sx+bounds.left,y:(mask.y+y)/sy+bounds.top,width:mask.width/sx,height:mask.height/sy}:null;
  }catch{return null;}
}
