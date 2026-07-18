async function n(t){try{return[null,await t().catch(r=>{throw r})]}catch(r){return[r,null]}}export{n as u};
