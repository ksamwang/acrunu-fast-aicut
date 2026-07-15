import {interpolate, useCurrentFrame, useVideoConfig} from "remotion";
import type {RenderSmokeProps} from "./Root";

export const RenderSmoke = ({title}: RenderSmokeProps) => {
  const frame = useCurrentFrame();
  const {durationInFrames} = useVideoConfig();
  const opacity = interpolate(frame, [0, 12, durationInFrames - 12, durationInFrames], [0, 1, 1, 0], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  return (
    <div
      style={{
        alignItems: "center",
        backgroundColor: "#10161a",
        color: "#f2f5f7",
        display: "flex",
        fontFamily: "Noto Sans CJK SC, sans-serif",
        fontSize: 58,
        height: "100%",
        justifyContent: "center",
        letterSpacing: 0,
        opacity,
        padding: 80,
        textAlign: "center",
        width: "100%",
      }}
    >
      {title}
    </div>
  );
};
