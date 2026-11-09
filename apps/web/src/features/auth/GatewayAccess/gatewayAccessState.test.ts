import { describe, expect, it } from "vitest";

import { ApiError } from "../../../shared/api";
import { gatewayAccessState } from "./gatewayAccessState";

describe("gatewayAccessState", () => {
  it("reads a first visit still being linked", () => {
    expect(gatewayAccessState(new ApiError(401, "access pending\n"))).toBe("pending");
  });

  it("reads a refusal", () => {
    expect(gatewayAccessState(new ApiError(403, "no access\n"))).toBe("refused");
  });

  it("reads a request without a gateway identity", () => {
    expect(gatewayAccessState(new ApiError(401, "unauthorized\n"))).toBe("outside");
    expect(gatewayAccessState(new Error("network"))).toBe("outside");
  });
});
