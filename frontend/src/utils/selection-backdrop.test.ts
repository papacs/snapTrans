import {describe,it,expect} from "vitest";
import {selectionBackdrop} from "./selection-backdrop";

function fixture(){
 const width=60,height=40,data=new Uint8ClampedArray(width*height*4).fill(255);
 const fill=(x:number,y:number,w:number,h:number,color:number[])=>{
  for(let py=y;py<y+h;py++)for(let px=x;px<x+w;px++)data.set([...color,255],(py*width+px)*4);
 };
 fill(10,8,34,24,[45,95,210]);
 fill(15,15,8,8,[255,255,255]);
 return {data,width,height,fill};
}
describe("native selection backdrop",()=>{
 it("covers line-box highlight margins without changing glyph coordinates",()=>{
  const f=fixture();
  expect(selectionBackdrop(f,{x:10,y:12,width:30,height:16},"#ffffff")).toEqual({x:10,y:8,width:34,height:24});
 });
 it("stops at different-colored adjacent content",()=>{
  const f=fixture();f.fill(44,8,3,24,[0,0,0]);
  expect(selectionBackdrop(f,{x:10,y:12,width:30,height:16},"#ffffff")?.width).toBe(34);
 });
 it("leaves an actual colored page background alone",()=>{
  const f=fixture();
  expect(selectionBackdrop(f,{x:10,y:12,width:30,height:16},"#2d5fd2")).toBeNull();
 });
 it("does not expand through text or invalid image data",()=>{
  const f=fixture();f.fill(10,11,30,1,[0,0,0]);f.fill(10,28,30,1,[0,0,0]);
  const result=selectionBackdrop(f,{x:10,y:12,width:30,height:16},"#ffffff");
  expect(result?.y).toBe(12);expect(result?.height).toBe(16);
  expect(selectionBackdrop({data:new Uint8ClampedArray(),width:60,height:40},{x:10,y:12,width:30,height:16},"#ffffff")).toBeNull();
 });
});
