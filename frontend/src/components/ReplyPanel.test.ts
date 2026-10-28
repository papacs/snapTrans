import { enableAutoUnmount, flushPromises, mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import ReplyPanel from "./ReplyPanel.vue";
import type { TextActionEvent, TextActionRequest } from "../services/backend";

const mocks=vi.hoisted(()=>({
  start:vi.fn(async(_request:TextActionRequest)=>{}),cancel:vi.fn(async(_id:string)=>{}),copy:vi.fn(async(_text:string)=>{}),
  listener:null as null|((event:TextActionEvent)=>void),
}));
vi.mock("../services/backend",()=>({startTextAction:mocks.start,cancelTextAction:mocks.cancel,copyText:mocks.copy,
  onBackendEvent:(_name:string,callback:(event:TextActionEvent)=>void)=>{mocks.listener=callback;return ()=>{mocks.listener=null;};},
}));
enableAutoUnmount(afterEach);
beforeEach(()=>{
  vi.clearAllMocks();sessionStorage.clear();
  vi.stubGlobal("requestAnimationFrame",vi.fn(()=>1));vi.stubGlobal("cancelAnimationFrame",vi.fn());
});
afterEach(()=>vi.unstubAllGlobals());
const props={locale:"zh-CN",anchor:{x:20,y:30,width:300,height:200},viewport:{width:1280,height:800},region:{x:50,y:70,width:600,height:400}};

describe("reply assistant",()=>{
  it("uses images when opted in and allows switching back to text",async()=>{
    const wrapper=mount(ReplyPanel,{props:{...props,directImageInput:true}});await flushPromises();
    const request=mocks.start.mock.calls[0]![0];
    expect(request).toMatchObject({inputMode:"image",region:props.region,text:""});
    expect(wrapper.text()).toContain("正在准备截图");
    mocks.listener!({id:request.id,stage:"writing",image:"data:image/jpeg;base64,preview",token:"回复草稿",done:true});await flushPromises();
    expect(wrapper.get('[data-testid="reply-image-preview"]').attributes("src")).toBe("data:image/jpeg;base64,preview");
    await wrapper.get('[data-testid="reply-image-mode"]').setValue(false);
    expect(wrapper.get('[data-testid="reply-copy-close"]').attributes("disabled")).toBeDefined();
    await wrapper.get('[data-testid="reply-generate"]').trigger("click");await flushPromises();
    expect(mocks.start.mock.calls[1]![0]).toMatchObject({inputMode:"ocr",region:props.region});
  });
  it("keeps text selections on text input even when direct images are enabled",async()=>{
    const wrapper=mount(ReplyPanel,{props:{...props,region:undefined,source:"Selected text",directImageInput:true}});await flushPromises();
    expect(mocks.start.mock.calls[0]![0]).toMatchObject({inputMode:"ocr",text:"Selected text"});
    expect(wrapper.find('[data-testid="reply-image-mode"]').exists()).toBe(false);
  });
  it("starts directly with the selected region and streams only its own request",async()=>{
    const wrapper=mount(ReplyPanel,{props});await flushPromises();
    const request=mocks.start.mock.calls[0]![0];
    expect(request).toMatchObject({action:"reply",region:props.region,text:"",scene:"general"});
    expect(wrapper.text()).toContain("正在识别文字");
    mocks.listener!({id:"old",source:"stale",token:"wrong",done:true});
    mocks.listener!({id:request.id,source:"Please confirm the priority",stage:"writing",done:false});
    mocks.listener!({id:request.id,token:"请确认一下优先级。",done:true});await flushPromises();
    expect((wrapper.get('[data-testid="reply-source"]').element as HTMLTextAreaElement).value).toBe("Please confirm the priority");
    expect((wrapper.get('[data-testid="reply-answer"]').element as HTMLTextAreaElement).value).toBe("请确认一下优先级。");
    expect(wrapper.get('[data-testid="reply-copy-close"]').attributes("disabled")).toBeUndefined();
    window.dispatchEvent(new KeyboardEvent("keydown",{key:"Enter",ctrlKey:true,bubbles:true}));await flushPromises();
    expect(mocks.copy).toHaveBeenCalledWith("请确认一下优先级。");expect(wrapper.emitted("copied")).toHaveLength(1);
  });
  it("preserves editable output and requires a fresh generation after intent changes",async()=>{
    const wrapper=mount(ReplyPanel,{props:{...props,source:"A work message"}});await flushPromises();
    let request=mocks.start.mock.calls[0]![0];mocks.listener!({id:request.id,token:"初稿",done:true});await flushPromises();
    await wrapper.get('[data-testid="reply-answer"]').setValue("我修改后的回复");
    await wrapper.get('[data-testid="reply-intent"]').setValue("礼貌拒绝，说明排期已满");
    await wrapper.get('[data-testid="reply-scene-work"]').trigger("click");
    expect(wrapper.get('[data-testid="reply-copy-close"]').attributes("disabled")).toBeDefined();
    await wrapper.get('[data-testid="reply-generate"]').trigger("click");await flushPromises();
    request=mocks.start.mock.calls[1]![0];expect(request).toMatchObject({scene:"work",intent:"礼貌拒绝，说明排期已满",text:"A work message"});
  });
  it("stops an old stream and ignores tokens after a new generation starts",async()=>{
    const wrapper=mount(ReplyPanel,{props:{...props,source:"message"}});await flushPromises();
    const first=mocks.start.mock.calls[0]![0];
    mocks.listener!({id:first.id,token:"部分回复",done:false});
    await wrapper.get('[data-testid="reply-stop"]').trigger("click");await flushPromises();
    expect(mocks.cancel).toHaveBeenCalledWith(first.id);
    await wrapper.get('[data-testid="reply-generate"]').trigger("click");await flushPromises();
    mocks.listener!({id:first.id,token:"过期内容",done:true});await flushPromises();
    expect((wrapper.get('[data-testid="reply-answer"]').element as HTMLTextAreaElement).value).toBe("");
    expect(wrapper.find('[data-testid="reply-stop"]').exists()).toBe(true);
  });
  it("cancels on close and surfaces clipboard failures without closing",async()=>{
    const wrapper=mount(ReplyPanel,{props:{...props,source:"message"}});await flushPromises();
    const request=mocks.start.mock.calls[0]![0];
    mocks.listener!({id:request.id,token:"回复",done:true});await flushPromises();
    mocks.copy.mockRejectedValueOnce(new Error("Clipboard is busy"));
    await wrapper.get('[data-testid="reply-copy-close"]').trigger("click");await flushPromises();
    expect(wrapper.text()).toContain("Clipboard is busy");expect(wrapper.emitted("copied")).toBeUndefined();
    await wrapper.get('[data-testid="reply-generate"]').trigger("click");await flushPromises();
    const pending=mocks.start.mock.calls[1]![0];wrapper.unmount();expect(mocks.cancel).toHaveBeenCalledWith(pending.id);
  });
});
