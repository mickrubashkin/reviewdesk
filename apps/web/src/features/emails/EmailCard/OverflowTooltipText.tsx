import { useEffect, useRef, useState } from "react";
import { Text, Tooltip } from "@mantine/core";

type OverflowTooltipTextProps = {
  c?: string;
  lineClamp: number;
  size: string;
  text: string;
};

export function OverflowTooltipText({
  c,
  lineClamp,
  size,
  text,
}: OverflowTooltipTextProps) {
  const textRef = useRef<HTMLParagraphElement>(null);
  const [isOverflowing, setIsOverflowing] = useState(false);

  useEffect(() => {
    const textElement = textRef.current;
    if (!textElement) {
      return;
    }

    const updateOverflow = () => {
      setIsOverflowing(
        textElement.scrollHeight > textElement.clientHeight ||
        textElement.scrollWidth > textElement.clientWidth
      );
    };

    updateOverflow();

    const resizeObserver = new ResizeObserver(updateOverflow);
    resizeObserver.observe(textElement);

    return () => resizeObserver.disconnect();
  }, [text]);

  return (
    <Tooltip
      disabled={!isOverflowing}
      label={text}
      multiline
      w={220}
      position="top-start"
      withArrow
    >
      <Text ref={textRef} size={size} c={c} lineClamp={lineClamp}>
        {text}
      </Text>
    </Tooltip>
  );
}
