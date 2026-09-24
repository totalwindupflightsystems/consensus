# React Review - Reference Documentation

## Common React Issues to Check

### Performance Issues
1. **Unnecessary re-renders**: Components re-rendering when props haven't changed
2. **Large bundle sizes**: Importing entire libraries instead of specific functions
3. **Memory leaks**: Missing cleanup in useEffect
4. **Expensive calculations**: Calculations in render without useMemo

### Best Practices
1. **Component structure**: One component per file, clear separation of concerns
2. **Prop patterns**: Avoid prop drilling, use context or composition
3. **State management**: Local state vs. global state, proper state lifting
4. **Type safety**: Proper TypeScript interfaces, no `any` types

### Anti-patterns
1. **useEffect misuse**: Missing dependencies, side effects in render
2. **Missing keys**: List items without proper keys
3. **Inline functions**: Creating new functions on every render
4. **Large components**: Components doing too much

## React Hooks Guidelines

### useState
- Initialize with proper type
- Use functional updates for derived state
- Avoid state that can be derived from props

### useEffect
- Include all dependencies
- Cleanup functions for subscriptions
- Avoid race conditions with cleanup

### useMemo / useCallback
- Use for expensive calculations
- Memoize callbacks passed to child components
- Don't over-optimize prematurely

## Resources

- [React Documentation](https://react.dev/)
- [React TypeScript Cheatsheet](https://react-typescript-cheatsheet.netlify.app/)
- [React Performance](https://react.dev/reference/react/memo)
