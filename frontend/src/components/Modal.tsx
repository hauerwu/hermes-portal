import { useEffect, useRef } from "react";
import { X } from "lucide-react";

/**
 * 通用模态窗口：
 *  - ESC / 右上角 × 关闭（点击外部不会关闭，避免误操作丢失表单内容）
 *  - body 滚动锁定，打开时聚焦首个可聚焦元素
 *  - role="dialog" + aria-modal，淡入/缩放动画
 */
export default function Modal({
  open,
  onClose,
  title,
  width = "max-w-md",
  children,
}: {
  open: boolean;
  onClose: () => void;
  title?: string;
  width?: string;
  children: React.ReactNode;
}) {
  const panelRef = useRef<HTMLDivElement>(null);

  // ESC 关闭
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  // body 滚动锁定
  useEffect(() => {
    if (!open) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, [open]);

  // 打开时聚焦首个可聚焦元素
  useEffect(() => {
    if (!open) return;
    const el = panelRef.current?.querySelector<HTMLElement>(
      "input:not([type=hidden]), select, textarea, button"
    );
    el?.focus();
  }, [open]);

  if (!open) return null;

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={title}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm animate-fade-in"
    >
      <div
        ref={panelRef}
        className={`w-full ${width} max-h-[85vh] overflow-y-auto rounded-xl border border-zinc-700 bg-zinc-900 shadow-2xl animate-modal-in`}
      >
        <div className="sticky top-0 z-10 flex items-center justify-between border-b border-zinc-800 bg-zinc-900/95 px-5 py-3.5 backdrop-blur">
          <h2 className="text-base font-semibold">{title}</h2>
          <button
            onClick={onClose}
            aria-label="关闭"
            className="rounded-md p-1.5 text-zinc-500 hover:bg-zinc-800 hover:text-zinc-200"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="p-5">{children}</div>
      </div>
    </div>
  );
}
