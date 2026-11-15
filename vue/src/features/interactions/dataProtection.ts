// 数据保护：仅在输入框/文本域内允许复制，防止整页学习数据被拷贝（隐私保护）。
export function preventDataCopy(event: ClipboardEvent) {
  if (
    !(event.target instanceof HTMLInputElement) &&
    !(event.target instanceof HTMLTextAreaElement)
  )
    event.preventDefault();
}
