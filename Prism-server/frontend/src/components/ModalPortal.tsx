import { createPortal } from 'react-dom';

/**
 * ModalPortal - 把内联弹窗渲染到 document.body 顶层。
 *
 * 玻璃主题下，页面容器 `.page-enter` / 极光背景 `body::before`（position:fixed
 * + filter:blur + 无限 transform 动画）会破坏内部 position:fixed 弹窗的 Chrome
 * 合成，导致弹窗主体不绘制。渲染到 body 顶层即可脱离该合成上下文 —— 与
 * Modal.tsx 用 createPortal 的效果一致。
 */
export default function ModalPortal({ children }: { children: React.ReactNode }) {
  return createPortal(children, document.body);
}
