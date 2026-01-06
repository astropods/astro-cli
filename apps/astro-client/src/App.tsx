import { useState } from 'react'
import reactLogo from './assets/react.svg'
import viteLogo from '/vite.svg'

function App() {
  const [count, setCount] = useState(0)

  return (
    <div className="w-full mx-auto p-8 text-center">
      <div className="flex justify-center gap-4">
        <a href="https://vite.dev" target="_blank">
          <img
            src={viteLogo}
            className="h-24 p-6 transition-all duration-300 hover:drop-shadow-[0_0_2em_#646cffaa]"
            alt="Vite logo"
          />
        </a>
        <a href="https://react.dev" target="_blank">
          <img
            src={reactLogo}
            className="h-24 p-6 transition-all duration-300 hover:drop-shadow-[0_0_2em_#61dafbaa] motion-safe:animate-[spin_20s_linear_infinite]"
            alt="React logo"
          />
        </a>
      </div>
      <h1 className="text-5xl font-bold my-4">Vite + React</h1>
      <div className="p-8">
        <button
          onClick={() => setCount((count) => count + 1)}
          className="px-5 py-2.5 text-base font-medium rounded-lg border border-transparent bg-neutral-800 hover:border-indigo-500 transition-colors cursor-pointer"
        >
          count is {count}
        </button>
        <p className="mt-4">
          Edit <code className="bg-neutral-800 px-1.5 py-0.5 rounded">src/App.tsx</code> and save to test HMR
        </p>
      </div>
      <p className="text-neutral-500">
        Click on the Vite and React logos to learn more
      </p>
    </div>
  )
}

export default App
