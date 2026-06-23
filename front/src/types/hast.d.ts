declare module 'hast' {
  export type Text = {
    type: 'text'
    value: string
  }

  export type Element = {
    type: 'element'
    tagName: string
    properties?: Record<string, unknown>
    children: Array<Element | Text>
  }
}
