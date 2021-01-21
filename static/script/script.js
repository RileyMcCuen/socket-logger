const colorClasses = [
    'debug',
    'info',
    'warn',
    'error'
];

const levels = [
    'DEBUG',
    'INFO ',
    'WARN ',
    'ERROR',
];

const listElem = document.getElementById("list");

const socket = new WebSocket('ws://localhost:9000/rec');

let showLevel = true;
let showFile = true;
let showTime = true;
let clearOnStart = false;
let clearOnFinish = false;

function clear() {
    [...document.getElementsByClassName('entry')].forEach(elem => {
        listElem.removeChild(elem);
    });
}

socket.onmessage = (ev) => {
    console.log(ev)
    const data = JSON.parse(ev.data);
    if ((clearOnStart && data.level === -2) || (clearOnFinish && data.level === -1)) {
        clear();
        return;
    }
    const li = document.createElement('li');
    li.classList.add(colorClasses[data.level], 'entry');
    if (data.file_name !== "" || data.line_num !== -1) {
        li.title = `${data.file_name} :: ${data.line_num} : ${data.column_num}`;
    } else {
        li.title = 'No file information provided';
    }
    li.innerHTML= `
        <div>
            <span class="${showLevel ? "level" : "level hide"}">[${levels[data.level]}]</span>
            <span class="${showFile ? "file" : "file hide"}">
                <<span class="file-name">${data.file_name}</span>@<span class="line-num">${data.line_num}</span>:<span class="column-num">${data.column_num}</span>>
            </span>  
            <span class="${showTime ? "time" : "time hide"}">(${new Date(data.time).toISOString()})</span>
        </div>
        <div class="content">
            ${data.content}
        </div>
    `;
    listElem.appendChild(li);
}

function toggleDisplay(className) {
    for(const elem of document.getElementsByClassName(className)) {
        elem.classList.toggle('hide');
    }
}

function onLevel(ev) {
    ev.target.classList.toggle('selected');
    showLevel = !showLevel;
    toggleDisplay('level');
}
function onFile(ev) {
    ev.target.classList.toggle('selected');
    showFile = !showFile;
    toggleDisplay('file');
}
function onTime(ev) {
    ev.target.classList.toggle('selected');
    showTime = !showTime;
    toggleDisplay('time');
}

document.getElementById('level-btn').addEventListener('click', onLevel);
document.getElementById('file-btn').addEventListener('click', onFile);
document.getElementById('time-btn').addEventListener('click', onTime);
document.getElementById('clear-btn').addEventListener('click', clear);
document.getElementById('clear-on-start-btn').addEventListener('click', () => clearOnStart = !clearOnStart);
document.getElementById('clear-on-finish-btn').addEventListener('click', () => clearOnFinish = !clearOnFinish);
