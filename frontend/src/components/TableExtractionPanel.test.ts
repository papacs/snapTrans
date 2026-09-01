import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import TableExtractionPanel from "./TableExtractionPanel.vue";

describe("TableExtractionPanel", () => {
  it("previews editable cells and requests both export formats", async () => {
    const wrapper = mount(TableExtractionPanel, {
      props: {
        cells: [["Name", "Score"], ["Alice", "92"]],
        loading: false,
        error: "",
        copied: "",
        locale: "zh-CN",
        panelStyle: {},
      },
    });
    expect(wrapper.find('[data-testid="table-preview"]').exists()).toBe(true);
    await wrapper.get('[data-testid="table-cell-1-0"]').setValue("Bob");
    expect(wrapper.emitted("update:cells")?.[0]?.[0]).toEqual([
      ["Name", "Score"], ["Bob", "92"],
    ]);
    await wrapper.get('[data-testid="copy-table-tsv"]').trigger("click");
    await wrapper.get('[data-testid="copy-table-markdown"]').trigger("click");
    expect(wrapper.emitted("copy")).toEqual([["tsv"], ["markdown"]]);
  });

  it("shows loading and error states without enabling export", () => {
    const wrapper = mount(TableExtractionPanel, {
      props: { cells: [], loading: true, error: "请重新框选", copied: "", locale: "zh-CN", panelStyle: {} },
    });
    expect(wrapper.find('[data-testid="table-extraction-loading"]').exists()).toBe(true);
    expect(wrapper.get('[role="alert"]').text()).toContain("重新框选");
    expect(wrapper.get('[data-testid="copy-table-tsv"]').attributes("disabled")).toBeDefined();
  });
});
