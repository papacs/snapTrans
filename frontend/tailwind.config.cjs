/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./index.html", "./src/**/*.{vue,ts}"],
  darkMode: "class",
  theme: {
    extend: {
      boxShadow: {
        floating: "0 18px 60px rgba(15, 23, 42, 0.32)"
      }
    }
  },
  plugins: []
};

