import {Composition} from "remotion";
import {RenderSmoke} from "./RenderSmoke";

export type RenderSmokeProps = {
  title: string;
  durationInFrames: number;
  fps: number;
  width: number;
  height: number;
};

export const Root = () => {
  return (
    <Composition<RenderSmokeProps>
      id="RenderSmoke"
      component={RenderSmoke}
      durationInFrames={90}
      fps={30}
      width={1280}
      height={720}
      defaultProps={{
        title: "AICUT renderer ready",
        durationInFrames: 90,
        fps: 30,
        width: 1280,
        height: 720,
      }}
      calculateMetadata={({props}) => ({
        durationInFrames: props.durationInFrames,
        fps: props.fps,
        width: props.width,
        height: props.height,
      })}
    />
  );
};
