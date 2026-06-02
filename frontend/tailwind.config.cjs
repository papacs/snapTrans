/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./index.html", "./src/**/*.{vue,ts}"],
  theme: {
    extend: {
      boxShadow: {
        floating: "0 18px 60px rgba(15, 23, 42, 0.32)"
      }
    }
  },
  plugins: []
};

